# json-to-go

## Windows
编译wasm
```bash
$env:GOOS = "js"; $env:GOARCH = "wasm"; go build -ldflags="-s -w" -o main.wasm cmd/wasm/main.go
```
```bash
tinygo build -o main.wasm -target wasm -opt=2 -no-debug ./cmd/wasm/main.go
```
手动安装tinygo和binaryen使用临时环境变量编译.
```bash
$env:PATH += ";C:\Program Files\tinygo\bin";$env:WASMOPT = "C:\Program Files\binaryen\bin\wasm-opt.exe"; tinygo build -o main.wasm -target wasm -opt=2 -no-debug ./cmd/wasm/main.go
```

编译cmd/http/main.go
```bash
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o app.exe cmd/http/main.go
```

## Introduction

将json直接转为go结构体，支持注释，自定义tag，中文属性，属性类型推断，属性合并等功能，并提供简单易用的静态web界面

## Web

在线地址(chrome): https://zzlgo.github.io/json-to-go

## Features

* 支持自定义tag
* 支持指针类型
* 支持结构体嵌套
* 支持注释，可在上一行或行尾
* 支持中文属性，属性名格式化，属性类型自动判断
* 支持数组内对象属性合并
* 基于wasm，提供简单易用的静态web界面

## Quick Start

[私有化部署](deploy.md) <br>
