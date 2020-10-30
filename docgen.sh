#!/bin/sh

GO_MODULE=github.com/SKAARHOJ/ibeam-core-go

function normalize_url {
    # Normalize provided URL. Removing double slashes
    echo "$1" | sed -E 's,([^:]/)/+,\1,g'
}

# Setup
rm -rf "${GO_DOC_HTML_OUTPUT:-godoc}"

# Extract Go module name from a Go module file
if [[ -z "$GO_MODULE" ]]; then
    if [[ -z "$1" ]]; then
      echo "Please specify a go module"
      exit
    else
        GO_MODULE=$1
    fi
fi


# URL path to Go package and module documentation
URL=$(normalize_url "http://${GO_DOC_HTTP:-localhost:6060}/pkg/$GO_MODULE/")

# Starting godoc server
echo "Starting godoc server..."
godoc -http="${GO_DOC_HTTP:-localhost:6060}" &
PID=$!

# Waiting for godoc server
while ! curl --fail --silent "$URL" 2>&1 >/dev/null; do
    sleep 0.1
done

# Download all documentation content from running godoc server
wget \
    --recursive \
    --no-verbose \
    --convert-links \
    --page-requisites \
    --adjust-extension \
    --execute=robots=off \
    --include-directories="/lib,/pkg/$GO_MODULE,/src/$GO_MODULE" \
    --exclude-directories="*" \
    --directory-prefix="${GO_DOC_HTML_OUTPUT:-godoc}" \
    --no-host-directories \
    "$URL"

# Generate a simple redirect
echo "<script>window.location.href = '/pkg/$GO_MODULE';</script>" > ${GO_DOC_HTML_OUTPUT:-godoc}/index.html

# Stop godoc server
kill -9 "$PID"
echo "Stopped godoc server"
echo "Go source code documentation generated under ${GO_DOC_HTML_OUTPUT:-godoc}"
