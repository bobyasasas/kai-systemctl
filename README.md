# kai-systemctl

`kai-systemctl` 是一个只管理自己创建的 systemd service unit 的小工具。它把 unit 文件限制在 `/etc/systemd/system` 下，并且只操作带有 `kai-` 前缀和 `X-Kai-Systemctl=managed` 标记的 `.service` 文件。

## 功能

- CLI 模式：默认命令就是 `kai-systemctl`
- Web 模式：`kai-systemctl host -host 0.0.0.0 -port 8080`
- 支持新建、删除、重命名、编辑 service unit
- 支持 `start`、`stop`、`restart`、`enable`、`disable`、`status`
- 限制 unit 名称，避免路径穿越和误操作系统 unit
- 一键安装脚本

## 安装

```sh
curl -fsSL https://raw.githubusercontent.com/bobyasasas/kai-systemctl/main/install.sh | sh
```

## CLI

```sh
kai-systemctl list

kai-systemctl new demo \
  -description "Demo service" \
  -exec "/usr/bin/python3 -m http.server 9000" \
  -workdir "/opt/demo" \
  -user root

kai-systemctl show demo
kai-systemctl edit demo -file ./demo.service
kai-systemctl rename demo api
kai-systemctl start api
kai-systemctl enable api
kai-systemctl delete api
```

用户传入 `demo`、`kai-demo`、`kai-demo.service` 都会规范化为：

```text
/etc/systemd/system/kai-demo.service
```

## Web

前台启动：

```sh
kai-systemctl host 0.0.0.0 -port 8080
```

浏览器访问：

```text
http://服务器IP:8080
```

按 `Ctrl+C` 停止 Web 服务。

## 权限

写入 `/etc/systemd/system` 和执行 `systemctl daemon-reload` 通常需要 root 权限。建议：

```sh
sudo kai-systemctl new demo -exec "/bin/sleep infinity"
sudo kai-systemctl host -host 0.0.0.0 -port 8080
```

## 开发

```sh
go test ./...
go build -o kai-systemctl ./cmd/kai-systemctl
```
