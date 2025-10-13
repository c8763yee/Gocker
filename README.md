# 操作
第一次使用
```bash
make check-deps && make && sudo make install
```

# Debug
你可以查看錯誤日誌
```bash
sudo journalctl -u gocker-daemon.service -f
```
或是創建的容器錯誤資訊
```bash
LATEST=$(ls -t /var/lib/gocker/containers/ | head -1)
sudo cat /var/lib/gocker/containers/$LATEST/init.log
```

# intro
目前操作傳輸
cli -> socket -> daemon

# TODO
目前修改進度：
1. ps 正常
2. run、exec 須更多測試
3. start還需完善
4. 其餘cmd功能還未與daemon client接上
5. rootfs中ebpf功能有錯誤 還未解決
