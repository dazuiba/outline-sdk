#!/bin/bash

# 本地测试 GitHub Actions 构建流程
# 用于验证打包逻辑是否正确

set -e

cd "$(dirname "$0")/x/examples/http2transport"

echo "🧹 Cleaning up old test builds..."
rm -rf test-output
mkdir -p test-output

platforms=(
  "linux:amd64"
  "linux:arm64"
  "darwin:amd64"
  "darwin:arm64"
  "windows:amd64"
)

for platform in "${platforms[@]}"; do
  GOOS="${platform%%:*}"
  GOARCH="${platform##*:}"

  if [ "$GOOS" = "windows" ]; then
    OUTPUT="http2transport-${GOOS}-${GOARCH}.exe"
    ARCHIVE="http2transport-${GOOS}-${GOARCH}.zip"
  else
    OUTPUT="http2transport-${GOOS}-${GOARCH}"
    ARCHIVE="http2transport-${GOOS}-${GOARCH}.tar.gz"
  fi

  echo ""
  echo "🔨 Building $GOOS/$GOARCH..."

  # Build binary
  GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags='-s -w' -o "$OUTPUT" .

  if [ ! -f "$OUTPUT" ]; then
    echo "❌ Build failed: $OUTPUT not created"
    exit 1
  fi

  echo "✅ Built: $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"

  # Package with config directory
  echo "📦 Packaging..."
  mkdir -p package
  cp "$OUTPUT" package/
  cp -r config package/

  if [ "$GOOS" = "windows" ]; then
    (cd package && zip -r "../test-output/$ARCHIVE" . > /dev/null)
  else
    tar -czf "test-output/$ARCHIVE" -C package .
  fi

  # Cleanup
  rm -rf package "$OUTPUT"

  echo "✅ Created: test-output/$ARCHIVE ($(du -h "test-output/$ARCHIVE" | cut -f1))"

  # Verify archive contents
  echo "📋 Archive contents:"
  if [ "$GOOS" = "windows" ]; then
    unzip -l "test-output/$ARCHIVE" | grep -E "(http2transport|config/)" | head -5
  else
    tar -tzf "test-output/$ARCHIVE" | head -10
  fi
done

echo ""
echo "✅ All builds completed successfully!"
echo ""
echo "📂 Test output directory:"
ls -lh test-output/
echo ""
echo "🧪 To test extraction:"
echo "  cd test-output"
echo "  tar -xzf http2transport-darwin-arm64.tar.gz -C test-extract"
echo "  ls -la test-extract/"
