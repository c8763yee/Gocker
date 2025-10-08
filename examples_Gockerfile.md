# Gockerfile 範例

本文件包含各種 Gockerfile 的使用範例。

## 範例 1: 基本範例

```dockerfile
# Gockerfile
FROM alpine:latest
RUN echo "Building custom image"
CMD ["echo", "Hello from Gocker!"]
```

建構命令：
```bash
gocker build -t myimage:v1 .
```

## 範例 2: 包含應用程式的映像

建立應用程式檔案 `app.sh`:
```bash
#!/bin/sh
echo "Hello from my custom app!"
```

建立 Gockerfile:
```dockerfile
FROM alpine:latest
RUN apk add --no-cache curl
COPY app.sh /app/app.sh
RUN chmod +x /app/app.sh
CMD ["/app/app.sh"]
```

建構命令：
```bash
gocker build -t myapp:latest .
```

## 範例 3: 使用 ENTRYPOINT

```dockerfile
FROM alpine:latest
ENTRYPOINT ["/bin/sh", "-c"]
CMD ["echo Hello"]
```

## 範例 4: 多個 RUN 指令

```dockerfile
FROM alpine:latest
RUN apk update
RUN apk add --no-cache curl wget
RUN echo "Setup complete" > /tmp/setup.txt
CMD ["cat", "/tmp/setup.txt"]
```

## 範例 5: 使用 ADD 指令

```dockerfile
FROM alpine:latest
ADD config.tar.gz /app/
CMD ["ls", "/app"]
```

## 支援的指令

- **FROM**: 指定基礎映像
- **RUN**: 執行命令
- **CMD**: 設定容器預設命令 (支援 JSON 陣列和 shell 格式)
- **ENTRYPOINT**: 設定容器入口點 (支援 JSON 陣列和 shell 格式)
- **COPY**: 複製檔案到映像
- **ADD**: 複製檔案到映像 (支援 tar 解壓縮)

## 注意事項

1. Gockerfile 必須以 FROM 指令開始
2. 註解以 # 開頭
3. 空行會被忽略
4. JSON 陣列格式使用雙引號，例如: `["echo", "hello"]`
5. Shell 格式直接寫命令，例如: `echo hello`
