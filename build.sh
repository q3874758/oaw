# OAW Build Script

echo "Building OAW..."

cd "$(dirname "$0")/.."

# Build
echo "Compiling..."
go build -o bin/oaw ./cmd/oaw

if [ $? -eq 0 ]; then
    echo "✓ Build successful!"
    echo "  Binary: bin/oaw"
else
    echo "✗ Build failed!"
    exit 1
fi

echo ""
echo "Usage:"
echo "  ./bin/oaw init     # 初始化"
echo "  ./bin/oaw start   # 启动服务"
echo "  ./bin/oaw status  # 查看状态"
echo "  ./bin/oaw --help  # 帮助"
