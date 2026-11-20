# goDisk

goDisk — Disk Health & Performance Monitor

Windows 磁盘健康检测与性能测试工具。支持查看 SMART 和设备信息、运行顺序与随机读写测试，以及回看测速历史。

从 [GitHub Releases](https://github.com/charley008/godisk/releases) 下载 Windows x64 ZIP，解压后启动 `goDisk.exe`。首次测速前按[试用说明](docs/PREVIEW.md)安装 Microsoft DiskSpd。

## 发布

更新 `mygo.json` 的版本号并提交，在对应提交上创建并推送同版本的标签。例如版本 `0.3.2` 对应标签 `v0.3.2`。

```powershell
git tag -a v0.3.2 -m "goDisk v0.3.2"
git push origin v0.3.2
```

GitHub Actions 在版本标签推送后运行测试，构建 Windows x64 便携 ZIP，并在 Releases 发布 ZIP 和 SHA-256 校验文件。普通分支推送不触发发布构建。
