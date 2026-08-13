# Mosona Manager

![Golang](https://img.shields.io/badge/-Golang%201.26-00acd7?style=flat-square&logo=go&logoColor=white)
![InfluxDB](https://img.shields.io/badge/-InfluxDB%202-351c76?style=flat-square&logo=influxdb)
![Postgres](https://img.shields.io/badge/-Postgres%2018-336791?style=flat-square&logo=postgresql&logoColor=black)
![Docker](https://img.shields.io/badge/-Docker-0091e2?style=flat-square&logo=docker&logoColor=white)

English | [Español](./README_es.md) | [简体中文](./README_zh_cn.md)

<div align="center" style="position: relative">
  <img alt="home" style="width: 49%" src="https://github.com/user-attachments/assets/6e486a04-647a-4201-a0d8-977d7994f832" />
  <img alt="terminal" style="width: 49%" src="https://github.com/user-attachments/assets/b164f3c5-ba03-4a31-a8dc-5c60d473af37" />
</div>

<br />

设计为面向团队/个人的项目管理服务器监控与终端管理工具，具有全面的项目权限控制和基于 Agent 与 SSH 的远程管理协议功能。

## 快速开始

请参阅 [Quickstart (Docs)](https://manager.mosona.cc/docs/quickstart) 了解安装和配置的详细说明。

## 功能
- **项目管理**：轻松创建和管理多个项目。
- **用户权限**：为每个项目分配并控制用户权限。
- **SSH 远程管理**：通过 SSH 安全管理服务器和终端。
- **灵活的连接模式**：支持基于 Agent 和无 Agent 的连接方式。
- **实时监控**：实时监测服务器性能和项目状态。
- **通知**：接收重要事件的警报和通知。
- **Web 界面**：用户友好的 Web 界面，便于导航和管理。
- **API 访问**：支持 RESTful API，便于与其他工具和服务集成。
- **日志和审计**：跟踪系统内部所有操作和变更。
- **公共页面**：通过可定制的公共仪表板共享实时系统状态。

## 连接模式

### 无 Agent 模式

> 这是主动（正向）连接。被管理服务器必须暴露出来，以便 Hub 可以连接。在公网环境中，被管理服务器必须具有公网 IP。

无需在被管理服务器上安装任何 Agent，简化部署并降低开销。

### Agent 模式（A/P）

可选的轻量级 Agent 可以部署在被管理服务器上。Agent 将其状态上报到中心服务器，或由中心服务器查询 Agent 的状态。

#### 1. 主动模式

这也是主动（正向）连接，如上所述。

#### 2. 被动模式

这是被动（反向）连接。Hub 必须暴露出来，以便 Agent 可以连接。在公网环境中，Hub 必须具有公网 IP。

## 截图

| 首页 | 终端 |
|---|---|
| ![home](https://github.com/user-attachments/assets/6e486a04-647a-4201-a0d8-977d7994f832) | ![terminal](https://github.com/user-attachments/assets/b164f3c5-ba03-4a31-a8dc-5c60d473af37) |

| 新服务器 | 监控 |
|---|---|
| ![new server](https://github.com/user-attachments/assets/60ea1cdb-ec2d-4e4c-95e1-4472fba3c39c) | ![monitor](https://github.com/user-attachments/assets/c586f397-a4e9-4fd1-a0ab-f753ed4bfe8b) |

| 公开页面 (M) | 公开页面 |
|---|---|
| ![public m](https://github.com/user-attachments/assets/96509eb5-b3ae-4223-8e18-9de5b77466f6) | ![public](https://github.com/user-attachments/assets/15611147-45c2-4358-b5f0-22a5c2fab547) |

| 个人资料 | 管理员 |
|---|---|
| ![profile](https://github.com/user-attachments/assets/2efcc2dc-e87d-4a95-a854-0b44b871a903) | ![admin](https://github.com/user-attachments/assets/3e93b6b7-a41f-4cc6-9b72-4e7419696c0c) |

## 更新日志

请查看 [CHANGELOG.md](./CHANGELOG.md) 了解更改历史。

## 社区

- [Discord](https://discord.gg/gmWzrXFXsB)
- [GitHub Discussions](https://github.com/mosona-labs/mosona-manager/discussions)

## 赞助

如果你觉得 Mosona Manager 有用，想支持其开发，请考虑成为赞助者。

- [GitHub Sponsors](https://github.com/sponsors/arsfy)

你的支持有助于维护和改进该项目。

<a href="https://github.com/sponsors/arsfy">
    <img src=".readme/sponsors.svg" alt="Sponsors" width="460" />
</a>

本项目能够持续发展，离不开以下赞助者的支持。

<table>
  <tr>
    <td align="center">
      <a href="https://github.com/Asahina1096">
        <img src="https://avatars.githubusercontent.com/u/52925955?s=52&v=4" width="50px;" alt="Asahina1096"/><br />
        Asahina1096
      </a>
    </td>
    <td align="center">
      <a href="https://github.com/DyAxy">
        <img src="https://avatars.githubusercontent.com/u/111729065?s=52&v=4" width="50px" alt="DyAxy"/>
      </a>
      <br />
      &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
      <a href="https://github.com/DyAxy">
        DyAxy
      </a>
      &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
    </td>
  </tr>
</table>

## 许可

本项目采用 MIT 许可证。详情请参阅 [LICENSE](./LICENSE) 文件。
