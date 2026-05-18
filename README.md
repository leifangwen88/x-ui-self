# x-ui-self

基于 [x-ui](https://github.com/vaxilu/x-ui) 的自用二开版本，仓库：[leifangwen88/x-ui-self](https://github.com/leifangwen88/x-ui-self)。

本二开版本更针对于中转做优化，支持多IP分组管理，多中转站间协同同步，支持营销类IP系统化管理

支持多协议多用户的 xray 面板。
# 功能介绍

- 系统状态监控
- 支持多用户多协议，网页可视化操作
- 支持的协议：vmess、vless、trojan、shadowsocks、dokodemo-door、socks、http
- 支持配置更多传输配置
- 流量统计，限制流量，限制到期时间
- 可自定义 xray 配置模板
- 支持 https 访问面板（自备域名 + ssl 证书）
- 支持一键SSL证书申请且自动续签
- 更多高级配置项，详见面板

# 安装 & 升级

安装与升级方式与上游 x-ui 一致：一键脚本、手动解压包、Docker 三种方式。本仓库使用 `main` 分支。

## 一键安装 / 升级（推荐）

在服务器上以 **root** 执行（安装与升级使用同一条命令）：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/leifangwen88/x-ui-self/main/install.sh)
```

指定版本安装或升级（将 `v0.x.x` 换为 [Releases](https://github.com/leifangwen88/x-ui-self/releases) 中的标签）：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/leifangwen88/x-ui-self/main/install.sh) 0.3.4
```

安装完成后，也可在服务器上使用管理命令 `x-ui`（如 `x-ui update` 更新面板）。

## 手动安装 & 升级

1. 从 https://github.com/leifangwen88/x-ui-self/releases 下载对应架构的最新压缩包（一般为 `amd64`）
2. 将压缩包上传到服务器 `/root/`，使用 **root** 登录后执行下列命令

> 若 CPU 架构不是 `amd64`，将命令中的 `amd64` 改为 `arm64` 等实际架构名。

```bash
cd /root/
rm -rf x-ui/ /usr/local/x-ui/ /usr/bin/x-ui
tar zxvf x-ui-linux-amd64.tar.gz
chmod +x x-ui/x-ui x-ui/bin/xray-linux-* x-ui/x-ui.sh
cp x-ui/x-ui.sh /usr/bin/x-ui
cp -f x-ui/x-ui.service /etc/systemd/system/
mv x-ui/ /usr/local/
systemctl daemon-reload
systemctl enable x-ui
systemctl restart x-ui
```

手动升级时：下载新版本压缩包后，重复上述步骤即可覆盖安装（注意先备份 `/etc/x-ui/` 等数据目录）。

## 使用 Docker 安装

> Docker 用法参考上游社区方案；二开版本建议自行构建镜像以保证与当前代码一致。

1. 安装 Docker

```shell
curl -fsSL https://get.docker.com | sh
```

2. 克隆本仓库并构建、运行

```shell
git clone https://github.com/leifangwen88/x-ui-self.git
cd x-ui-self
docker build -t x-ui-self .
docker run -itd --network=host \
    -v $PWD/db/:/etc/x-ui/ \
    -v $PWD/cert/:/root/cert/ \
    --name x-ui --restart=unless-stopped \
    x-ui-self
```

> 第三方镜像 `enwaiax/x-ui` 等来自上游生态，与本 fork 代码可能不一致，生产环境请优先使用上面自行 `docker build` 的镜像。

## 发布 Release（维护者）

服务器一键安装/升级会从 **本仓库** 的 [Releases](https://github.com/leifangwen88/x-ui-self/releases) 下载 `x-ui-linux-{amd64,arm64,s390x}.tar.gz`，不会再去拉取上游 `vaxilu/x-ui`。

1. 在 GitHub 仓库 **Settings → Secrets → Actions** 中配置 `GITHUB_TOKEN`（或使用仓库自带的 `GITHUB_TOKEN` 权限；工作流已使用标准密钥名）。
2. 推送符合 `0.*` 格式的标签（如 `0.3.3`），将触发 `.github/workflows/release.yml` 自动构建并上传安装包。
3. 也可在 Releases 页面 **手动上传** 与脚本同名的压缩包（需先本地按工作流方式打包）。
4. 修改安装源仓库时，同步更新 `install.sh` 与 `x-ui.sh` 顶部的 `GITHUB_REPO`、`GITHUB_BRANCH` 常量。

> **注意**：在发布首个 Release 之前，一键安装会因找不到安装包而失败；开发阶段可在服务器上 `git clone` 后本地 `go build`，或先打标签触发 CI 发布。

## SSL证书申请

> 此功能与教程由[FranzKafkaYu](https://github.com/FranzKafkaYu)提供

脚本内置SSL证书申请功能，使用该脚本申请证书，需满足以下条件:

- 知晓Cloudflare 注册邮箱
- 知晓Cloudflare Global API Key
- 域名已通过cloudflare进行解析到当前服务器

获取Cloudflare Global API Key的方法:
    ![](media/bda84fbc2ede834deaba1c173a932223.png)
    ![](media/d13ffd6a73f938d1037d0708e31433bf.png)

使用时只需输入 `域名`, `邮箱`, `API KEY`即可，示意图如下：
        ![](media/2022-04-04_141259.png)

注意事项:

- 该脚本使用DNS API进行证书申请
- 默认使用Let'sEncrypt作为CA方
- 证书安装目录为/root/cert目录
- 本脚本申请证书均为泛域名证书

## Tg机器人使用（开发中，暂不可使用）

> 此功能与教程由[FranzKafkaYu](https://github.com/FranzKafkaYu)提供

X-UI支持通过Tg机器人实现每日流量通知，面板登录提醒等功能，使用Tg机器人，需要自行申请
具体申请教程可以参考[博客链接](https://coderfan.net/how-to-use-telegram-bot-to-alarm-you-when-someone-login-into-your-vps.html)
使用说明:在面板后台设置机器人相关参数，具体包括

- Tg机器人Token
- Tg机器人ChatId
- Tg机器人周期运行时间，采用crontab语法  

参考语法：
- 30 * * * * * //每一分的第30s进行通知
- @hourly      //每小时通知
- @daily       //每天通知（凌晨零点整）
- @every 8h    //每8小时通知  

TG通知内容：
- 节点流量使用
- 面板登录提醒
- 节点到期提醒
- 流量预警提醒  

更多功能规划中...
## 建议系统

- CentOS 7+
- Ubuntu 16+
- Debian 8+

# 常见问题

## 从 v2-ui 迁移

首先在安装了 v2-ui 的服务器上安装最新版 x-ui，然后使用以下命令进行迁移，将迁移本机 v2-ui 的 `所有 inbound 账号数据`至 x-ui，`面板设置和用户名密码不会迁移`

> 迁移成功后请 `关闭 v2-ui`并且 `重启 x-ui`，否则 v2-ui 的 inbound 会与 x-ui 的 inbound 会产生 `端口冲突`

```
x-ui v2-ui
```


## Stargazers over time

[![Stargazers over time](https://starchart.cc/leifangwen88/x-ui-self.svg)](https://starchart.cc/leifangwen88/x-ui-self)
