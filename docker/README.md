# [zoraxy](https://github.com/tobychui/zoraxy/) </br>

[![Repo](https://img.shields.io/badge/Docker-Repo-007EC6?labelColor-555555&color-007EC6&logo=docker&logoColor=fff&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)
[![Version](https://img.shields.io/docker/v/zoraxydocker/zoraxy/latest?labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)
[![Size](https://img.shields.io/docker/image-size/zoraxydocker/zoraxy/latest?sort=semver&labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)
[![Pulls](https://img.shields.io/docker/pulls/zoraxydocker/zoraxy?labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)

## Setup: </br>
Although not required, it is recommended to give Zoraxy a dedicated location on the host to mount the container. That way, the host/user can access them whenever needed. A volume will be created automatically within Docker if a location is not specified. </br>

You may also need to portforward your 80/443 to allow http and https traffic. If you are accessing the interface from outside of the local network, you may also need to forward your management port. If you know how to do this, great! If not, find the manufacturer of your router and search on how to do that. There are too many to be listed here. </br>

The examples below are not exactly how it should be set up, rather they give a general idea of usage.

### Using Docker run </br>
```
docker run -d --name (container name) -p 80:80 -p 443:443 -p (management external):(management internal) -v (path to storage directory):/opt/zoraxy/data/ -e (flag)="(value)" zoraxydocker/zoraxy:latest
```

### Using Docker Compose </br>
```yml
services:
  zoraxy-docker:
    image: zoraxydocker/zoraxy:latest
    container_name: (container name)
    ports:
      - 80:80
      - 443:443
      - (management external):(management internal)
    volumes:
      - (path to storage directory):/opt/zoraxy/config/
    environment:
      (flag): "(value)"
```

| Operator | Need | Details |
|:-|:-|:-|
| `-d` | Yes | will run the container in the background. |
| `--name (container name)` | No | Sets the name of the container to the following word. You can change this to whatever you want. |
| `-p (ports)` | Yes | Depending on how your network is setup, you may need to portforward 80, 443, and the management port. |
| `-v (path to storage directory):/opt/zoraxy/config/` | Recommend | Sets the folder that holds your files. This should be the place you just chose. By default, it will create a Docker volume for the files for persistency but they will not be accessible. |
| `-v /var/run/docker.sock:/var/run/docker.sock` | No | Used for autodiscovery. |
| `-e (flag)="(value)"` | No | Arguments to run Zoraxy with. They are simply just capitalized Zoraxy flags. `-docker=true` is always set by default. See examples below. |
| `zoraxydocker/zoraxy:latest` | Yes | The repository on Docker hub. By default, it is the latest version that is published. |

> [!IMPORTANT]
> Docker usage of the port flag should not include the colon. Ex: PORT="8000"

## Examples: </br>
### Docker Run </br>
```
docker run -d --name zoraxy -p 80:80 -p 443:443 -p 8005:8005 -v /home/docker/Containers/Zoraxy:/opt/zoraxy/config/ -v /var/run/docker.sock:/var/run/docker.sock -e PORT="8005" -e FASTGEOIP="true" zoraxydocker/zoraxy:latest
```

### Docker Compose </br>
```yml
services:
  zoraxy-docker:
    image: zoraxydocker/zoraxy:latest
    container_name: zoraxy
    ports:
      - 80:80
      - 443:443
      - 8005:8005
    volumes:
      - /home/docker/Containers/Zoraxy:/opt/zoraxy/config/
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      PORT: "8005"
      FASTGEOIP: "true"
```
