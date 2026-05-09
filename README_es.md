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

Diseñado como una herramienta de gestión de servidores y terminales centrada en equipos/personas, con control integral de permisos de proyecto y un protocolo de gestión remota basado en Agentes y SSH.

## Funcionalidades
- **Gestión de proyectos**: crea y administra múltiples proyectos con facilidad.
- **Permisos de usuario**: asigna y controla permisos de usuarios por proyecto.
- **Gestión remota SSH**: administra servidores y terminales de forma segura mediante SSH.
- **Modos de conexión flexibles**: admite métodos de conexión basados en agente y sin agente.
- **Monitoreo en tiempo real**: supervisa el rendimiento del servidor y el estado del proyecto en tiempo real.
- **Notificaciones**: recibe alertas y notificaciones de eventos importantes.
- **Interfaz web**: interfaz web fácil de usar para una navegación y gestión sencillas.
- **Acceso API**: API RESTful para integración con otras herramientas y servicios.
- **Registro y auditoría**: realiza un seguimiento de todas las acciones y cambios dentro del sistema.
- **Página pública**: comparte el estado del sistema en tiempo real con un panel público personalizable.

## Modos de conexión

### Modo sin agente

> Esta es una conexión activa (directa). El servidor gestionado debe estar expuesto para que pueda ser conectado por el Hub. En un entorno de red pública, el servidor gestionado debe tener una dirección IP pública.

No es necesario instalar ningún agente en los servidores gestionados, lo que simplifica la implementación y reduce la sobrecarga.

### Modo agente (A/P)

Los agentes ligeros opcionales se pueden desplegar en los servidores gestionados. El Agente informa su estado al servidor central o el servidor central consulta el Agente por el estado.

#### 1. Modo activo

También es una conexión activa (directa), como se describió anteriormente.

#### 2. Modo pasivo

Esta es una conexión pasiva (inversa). El Hub debe estar expuesto para que el Agente pueda conectarse. En un entorno de red pública, el Hub debe tener una dirección IP pública.

## Comenzar e implementar

> En progreso

~~Por favor consulta la [Guía rápida (Docs)](https://manager.mosona.cc/quickstart) para obtener instrucciones detalladas sobre instalación y configuración.~~

## Capturas de pantalla

| Inicio | Terminal |
|---|---|
| ![home](https://github.com/user-attachments/assets/6e486a04-647a-4201-a0d8-977d7994f832) | ![terminal](https://github.com/user-attachments/assets/b164f3c5-ba03-4a31-a8dc-5c60d473af37) |

| Nuevo servidor | Monitor |
|---|---|
| ![new server](https://github.com/user-attachments/assets/60ea1cdb-ec2d-4e4c-95e1-4472fba3c39c) | ![monitor](https://github.com/user-attachments/assets/c586f397-a4e9-4fd1-a0ab-f753ed4bfe8b) |

| Página pública (M) | Página pública |
|---|---|
| ![public m](https://github.com/user-attachments/assets/96509eb5-b3ae-4223-8e18-9de5b77466f6) | ![public](https://github.com/user-attachments/assets/15611147-45c2-4358-b5f0-22a5c2fab547) |

| Perfil | Administrador |
|---|---|
| ![profile](https://github.com/user-attachments/assets/2efcc2dc-e87d-4a95-a854-0b44b871a903) | ![admin](https://github.com/user-attachments/assets/3e93b6b7-a41f-4cc6-9b72-4e7419696c0c) |

## Changelog

Consulta [CHANGELOG.md](./CHANGELOG.md) para el historial de cambios.

## Comunidad

- [Discord](https://discord.gg/NzKFaZGe)
- [GitHub Discussions](https://github.com/mosona-labs/mosona-manager/discussions)

## Patrocinadores

Si encuentras Mosona Manager útil y deseas apoyar su desarrollo, considera convertirte en patrocinador.

- [GitHub Sponsors](https://github.com/sponsors/arsfy)

Tu apoyo ayuda a mantener y mejorar el proyecto.

<a href="https://github.com/sponsors/arsfy">
    <img src=".readme/sponsors.svg" alt="Sponsors" width="460" />
</a>

Este proyecto es posible gracias al apoyo de los siguientes patrocinadores.

<table>
  <tr>
    <td align="center"><a href="https://github.com/Asahina1096"><img src="https://avatars.githubusercontent.com/u/52925955?s=52&v=4" width="50px;" alt="Asahina1096"/><br />Asahina1096</a></td>
  </tr>
</table>

## Licencia

Este proyecto está licenciado bajo la Licencia MIT. Consulta el archivo [LICENSE](./LICENSE) para más detalles.
