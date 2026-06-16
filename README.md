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

Designed as a team-oriented / personal project management server monitor and terminal management tool, featuring comprehensive project permission control and Agent & SSH-driven remote management protocol.

## Features
- **Project Management**: Create and manage multiple projects with ease.
- **User Permissions**: Assign and control user permissions for each project.
- **SSH Remote Management**: Securely manage servers and terminals via SSH.
- **Flexible Connection Modes**: Support both agent-based and agentless connection methods.
- **Real-time Monitoring**: Monitor server performance and project status in real-time.
- **Notifications**: Receive alerts and notifications for important events.
- **Web Interface**: User-friendly web interface for easy navigation and management.
- **API Access**: RESTful API for integration with other tools and services.
- **Logging and Auditing**: Keep track of all actions and changes within the system.
- **Public Page**: Share real-time system status with a customizable public-facing dashboard.

## Connection Modes

### Agentless Mode

> This is an active (forward) connection. The managed server must be exposed so that it can be connected by the Hub. In a public network environment, the managed server must have a public IP address.

No need to install any agents on the managed servers, simplifying deployment and reducing overhead.

### Agent Mode (A/P)

Optional lightweight agents can be deployed on managed servers. The Agent reports its status to the central server, or the central server queries the Agent for status.

#### 1. Active Mode

It is also an active (forward) connection, as detailed above.

#### 2. Passive Mode

This is a passive (reverse) connection. The Hub must be exposed so that it can be connected by the Agent. In a public network environment, the Hub must have a public IP address.

## Get started & Deploy

> In progress

~~Please refer to the [Quickstart (Docs)](https://manager.mosona.cc/quickstart) for detailed instructions on installation and configuration.~~

## Screenshots

| Home | Terminal |
|---|---|
| ![home](https://github.com/user-attachments/assets/6e486a04-647a-4201-a0d8-977d7994f832) | ![terminal](https://github.com/user-attachments/assets/b164f3c5-ba03-4a31-a8dc-5c60d473af37) |

| New Serevr | Monitor |
|---|---|
| ![new server](https://github.com/user-attachments/assets/60ea1cdb-ec2d-4e4c-95e1-4472fba3c39c) | ![monitor](https://github.com/user-attachments/assets/c586f397-a4e9-4fd1-a0ab-f753ed4bfe8b) |

| Public Page (M) | Public Page |
|---|---|
| ![public m](https://github.com/user-attachments/assets/96509eb5-b3ae-4223-8e18-9de5b77466f6) | ![public](https://github.com/user-attachments/assets/15611147-45c2-4358-b5f0-22a5c2fab547) |

| Profile | Admin |
|---|---|
| ![profile](https://github.com/user-attachments/assets/2efcc2dc-e87d-4a95-a854-0b44b871a903) | ![admin](https://github.com/user-attachments/assets/3e93b6b7-a41f-4cc6-9b72-4e7419696c0c) |

## Changelog

See [CHANGELOG.md](./CHANGELOG.md) for history of changes.

## Community

- [Discord](https://discord.gg/gmWzrXFXsB)
- [GitHub Discussions](https://github.com/mosona-labs/mosona-manager/discussions)

## Sponsors

If you find Mosona Manager helpful and would like to support its development, consider becoming a sponsor. 

- [GitHub Sponsors](https://github.com/sponsors/arsfy)

Your support helps us maintain and improve the project.

<a href="https://github.com/sponsors/arsfy">
    <img src=".readme/sponsors.svg" alt="Sponsors" width="460" />
</a>

This project is made possible thanks to the support of the following sponsors.

<table>
  <tr>
    <td align="center"><a href="https://github.com/Asahina1096"><img src="https://avatars.githubusercontent.com/u/52925955?s=52&v=4" width="50px;" alt="Asahina1096"/><br />Asahina1096</a></td>
  </tr>
</table>

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.
