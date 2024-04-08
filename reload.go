//go:build !windows
// +build !windows

package ibeamcorelib

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	log "github.com/s00500/env_logger"
)

// The Double-exec strategy: parent forks child to exec (first) with an
// inherited net.Listener; child signals parent to exec (second); parent
// kills child.

const (
	SIGINT  = syscall.SIGINT
	SIGQUIT = syscall.SIGQUIT
	SIGTERM = syscall.SIGTERM
	SIGUSR2 = syscall.SIGUSR2
)

// Re-exec this same image
func execReload() error {
	var pid int
	fmt.Sscan(os.Getenv("CORERELOAD_PID"), &pid)
	if syscall.Getppid() == pid {
		return fmt.Errorf("reload.Exec called by a child process")
	}
	argv0, err := lookPath()
	if nil != err {
		return err
	}

	if err := os.Setenv("CORERELOAD_SIGNAL", fmt.Sprintf("%d", syscall.SIGTERM)); nil != err {
		return err
	}

	if err := os.Setenv("CORERELOAD_CHILD", "0"); nil != err {
		return err
	}

	log.Traceln("re-executing", argv0)
	return syscall.Exec(argv0, os.Args, os.Environ())
}

// Fork and exec this same image
func forkExec() error {
	argv0, err := lookPath()
	if nil != err {
		return err
	}
	wd, err := os.Getwd()
	if nil != err {
		return err
	}

	if err := os.Setenv("CORERELOAD_PID", ""); nil != err {
		return err
	}
	if err := os.Setenv(
		"CORERELOAD_PPID",
		fmt.Sprint(syscall.Getpid()),
	); nil != err {
		return err
	}
	sig := syscall.SIGUSR2

	if err := os.Setenv("CORERELOAD_SIGNAL", fmt.Sprintf("%d", sig)); nil != err {
		return err
	}

	if err := os.Setenv("CORERELOAD_CHILD", "1"); nil != err {
		return err
	}

	files := make([]*os.File, 3)
	files[syscall.Stdin] = os.Stdin
	files[syscall.Stdout] = os.Stdout
	files[syscall.Stderr] = os.Stderr

	p, err := os.StartProcess(argv0, os.Args, &os.ProcAttr{
		Dir:   wd,
		Env:   os.Environ(),
		Files: files,
		Sys:   &syscall.SysProcAttr{},
	})
	if nil != err {
		return err
	}
	log.Debugln("spawned child", p.Pid)
	if err = os.Setenv("CORERELOAD_PID", fmt.Sprint(p.Pid)); nil != err {
		return err
	}
	return nil
}

// kill process specified in the environment with the signal specified in the
// environment; default to SIGQUIT.
func kill() error {
	var (
		pid int
		sig syscall.Signal
	)
	_, err := fmt.Sscan(os.Getenv("CORERELOAD_PID"), &pid)
	if err == io.EOF {
		_, err = fmt.Sscan(os.Getenv("CORERELOAD_PPID"), &pid)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Sscan(os.Getenv("CORERELOAD_SIGNAL"), &sig); nil != err {
		sig = syscall.SIGTERM
	}
	if syscall.SIGTERM == sig {
		go syscall.Wait4(pid, nil, 0, nil)
	}
	log.Traceln("sending signal", sig, "to process", pid)
	return syscall.Kill(pid, sig)
}

func ReloadHook() {
	if isChild() {
		log.Info("Reloading Core...")
		log.Trace("is childprocess, kill parent")
		err := kill()
		log.MustFatal(err)
		os.Exit(0)
	}
}

func isChild() bool {
	return os.Getenv("CORERELOAD_CHILD") == "1"
}

// Block this goroutine awaiting signals.  Signals are handled as they
// are by Nginx and Unicorn: <http://unicorn.bogomips.org/SIGNALS.html>.
func wait() (syscall.Signal, error) {
	ch := make(chan os.Signal, 2)
	signal.Notify(
		ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		//syscall.SIGQUIT,
		syscall.SIGTERM,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	)
	forked := false
	for {
		sig := <-ch
		switch sig {

		// SIGHUP should reload configuration.
		case syscall.SIGHUP:
			// FIXME: make sure to foward all sigs to the caller ?
			//if nil != OnSIGHUP {
			//	if err := OnSIGHUP(l); nil != err {
			//		log.Println("OnSIGHUP:", err)
			//	}
			//}

		// SIGINT should exit.
		case syscall.SIGINT:
			return syscall.SIGINT, nil

		// SIGQUIT should exit gracefully.
		case syscall.SIGQUIT:
			return syscall.SIGQUIT, nil

		// SIGTERM should exit.
		case syscall.SIGTERM:
			return syscall.SIGTERM, nil

		// SIGUSR1 should reopen logs.
		case syscall.SIGUSR1:
			// FIXME: make sure to foward all sigs to the caller ?
			//if nil != OnSIGUSR1 {
			//	if err := OnSIGUSR1(l); nil != err {
			//		log.Println("OnSIGUSR1:", err)
			//	}
			//}

		// SIGUSR2 forks and re-execs the first time it is received and execs
		// without forking from then on.
		case syscall.SIGUSR2:
			if forked {
				return syscall.SIGUSR2, nil
			}
			forked = true
			if err := forkExec(); nil != err {
				return syscall.SIGUSR2, err
			}

		}
	}
}

func lookPath() (argv0 string, err error) {
	argv0, err = exec.LookPath(os.Args[0])
	if nil != err {
		return
	}
	if _, err = os.Stat(argv0); nil != err {
		return
	}
	return
}
