# Mosona Manager

![Golang](https://img.shields.io/badge/-Golang%201.25-00acd7?style=flat-square&logo=go&logoColor=white)
![InfluxDB](https://img.shields.io/badge/-InfluxDB%202-351c76?style=flat-square&logo=influxdb)
![Postgres](https://img.shields.io/badge/-Postgres%2018-336791?style=flat-square&logo=postgresql&logoColor=black)
![Docker](https://img.shields.io/badge/-Docker-0091e2?style=flat-square&logo=docker&logoColor=white)

English | [Español](./README_es.md) | [简体中文](./README_zh_cn.md)

<div align="center" style="position: relative">
  <img alt="home" style="width: 49%" src="https://github.com/user-attachments/assets/0a27bf17-f40c-4fcd-903b-cae9ff9691bb" />
  <img alt="terminal" style="width: 49%" src="https://github.com/user-attachments/assets/420913b5-db7d-46f3-9942-4cc66c5a728f" />
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

Please refer to the [Quickstart (Docs)](https://manager.mosona.cc/quickstart) for detailed instructions on installation and configuration.

## Project Logo

<img src="./images/about.webp" alt="Mosona Manager" width="240" height="240">

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.