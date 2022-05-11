package syncmap

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}

func generateArrayOfRandomStrings(n int) []string {
	s := make([]string, n)
	for i := 0; i < n; i++ {
		s[i] = randomString(16)
	}
	return s
}

func Test_SyncMap_GetSet(t *testing.T) {
	sm := New[string, string]()

	if sm.Has("a key not in the map") {
		t.Fatal("Has() returned true on empty map")
	}

	// Set a value
	sm.Set("key a", "value a")
	if sm.Get("key a") != "value a" {
		t.Fatalf("Get(...) didn't return the right value")
	}

	// Change a value
	sm.Set("key a", "value b")
	if sm.Get("key a") == "value a" {
		t.Fatalf("Get(...) didn't return the right value")
	}

	// Extended change a value
	ss := generateArrayOfRandomStrings(20)
	for _, s := range ss {
		sm.Set("key b", s)
	}
	if sm.Get("key b") != ss[len(ss)-1] {
		t.Fatalf("Get(...) didn't return the right value")
	}
}
