# SearchEngine

基于 tantivy-go 的全文搜索引擎封装。

## 使用前准备

1. 在 `ferret/compute-node` 目录中运行：
   ```bash
   go get github.com/anyproto/tantivy-go
   ```

2. 从 [https://winlibs.com/](https://winlibs.com/) 下载 **GCC 16.1.0 (with POSIX threads) + MinGW-w64 14.0.0 (UCRT)**。

3. 将下载的压缩包解压到任意目录，并将 `mingw64/bin` 添加到系统的 `PATH` 环境变量。

4. 在 `compute-node` 项目根目录中运行：
   ```bash
   go get github.com/anyproto/tantivy-go@v1.0.6
   ```

5. 从 [https://github.com/anyproto/tantivy-go/releases](https://github.com/anyproto/tantivy-go/releases) 下载 `windows-amd64.tar.gz` 并解压。

6. 将解压得到的 `libtantivy_go.a` 文件放入 `mingw64\lib` 目录。

7. 编译项目：
   ```bash
   go build -o main.exe .\main.go
   ```
