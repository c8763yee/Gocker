# Gocker Build 命令

## 簡介

`gocker build` 命令允許您從 Gockerfile 建構自訂映像。Gockerfile 是一個文本文件，包含了建構映像所需的指令。

## 使用方式

```bash
gocker build [OPTIONS] PATH
```

### 選項

- `-t, --tag <name:tag>`: 指定映像名稱和標籤（必需）
- `-f, --file <path>`: 指定 Gockerfile 路徑（預設：PATH/Gockerfile）

## Gockerfile 語法

Gockerfile 支援以下指令：

### FROM

指定基礎映像。這是 Gockerfile 中的第一個指令。

```dockerfile
FROM alpine:latest
```

### RUN

在映像中執行命令。常用於安裝軟體包或執行設定腳本。

```dockerfile
RUN apk add --no-cache curl
RUN echo "Hello World" > /hello.txt
```

### CMD

設定容器啟動時的預設命令。

```dockerfile
CMD ["echo", "Hello from Gocker!"]
```

### ENTRYPOINT

設定容器的入口點。

```dockerfile
ENTRYPOINT ["/bin/sh"]
```

### COPY

從建構上下文複製檔案到映像中。

```dockerfile
COPY app.js /app/app.js
```

### ADD

與 COPY 類似，但支援 URL 和自動解壓縮功能。

```dockerfile
ADD https://example.com/file.tar.gz /app/
```

## 範例

### 基本範例

建立一個簡單的 Gockerfile：

```dockerfile
# Gockerfile
FROM alpine:latest
RUN echo "Building custom image"
CMD ["echo", "Hello from Gocker!"]
```

建構映像：

```bash
gocker build -t myimage:v1 .
```

### 完整範例

```dockerfile
# Gockerfile
FROM alpine:latest

# 安裝必要的軟體包
RUN apk add --no-cache curl bash

# 複製應用程式檔案
COPY app.sh /app/app.sh

# 設定工作目錄的權限
RUN chmod +x /app/app.sh

# 設定預設命令
CMD ["/app/app.sh"]
```

建構映像：

```bash
gocker build -t myapp:latest .
```

使用自訂 Gockerfile 路徑：

```bash
gocker build -t myapp:latest -f /path/to/Gockerfile /path/to/context
```

## 注意事項

1. Gockerfile 必須包含至少一個 FROM 指令
2. 建構映像前，請確保基礎映像已經存在或可以被拉取
3. RUN 指令在 chroot 環境中執行，需要系統支援
4. 建構過程會自動更新映像清單（manifest.json）
5. 每個指令都會記錄在建構日誌中

## 錯誤處理

如果建構失敗，請檢查：

- Gockerfile 語法是否正確
- 基礎映像是否存在
- 檔案路徑是否正確
- 是否有足夠的權限執行 chroot 命令

## 查看建構的映像

建構完成後，使用以下命令查看映像列表：

```bash
gocker images
```

## 執行建構的映像

```bash
gocker run myimage:v1
```
