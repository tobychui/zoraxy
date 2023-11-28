# [zoraxy](https://github.com/tobychui/zoraxy/) </br>

[![Repo](https://img.shields.io/badge/Docker-Repo-007EC6?labelColor-555555&color-007EC6&logo=docker&logoColor=fff&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)
[![Version](https://img.shields.io/docker/v/zoraxydocker/zoraxy/latest?labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)
[![Size](https://img.shields.io/docker/image-size/zoraxydocker/zoraxy/latest?sort=semver&labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)
[![Pulls](https://img.shields.io/docker/pulls/zoraxydocker/zoraxy?labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/zoraxydocker/zoraxy)

## Setup: </br>
Although not required, it is recommended to give Zoraxy a dedicated location on the host to mount the container. That way, the host/user can access them whenever needed. A volume will be created automatically within Docker if a location is not specified. </br>

You may also need to portforward your 80/443 to allow http and https traffic. If you are accessing the interface from outside of the local network, you may also need to forward your management port. If you know how to do this, great! If not, find the manufacturer of your router and search on how to do that. There are too many to be listed here. </br>

### Using Docker run </br>
```
docker run -d --name (container name) -p (ports) -v (path to storage directory):/opt/zoraxy/data/ -e ARGS='(your arguments)' zoraxydocker/zoraxy:latest
```

### Using Docker Compose </br>
```yml
version: '3.3'
services:
  zoraxy-docker:
    image: zoraxydocker/zoraxy:latest
    container_name: (container name)
    ports:
      - 80:80
      - 443:443
      - (external):8000
    volumes:
      - (path to storage directory):/opt/zoraxy/config/
    environment:
      ARGS: '(your arguments)'
```

| Operator | Need | Details |
|:-|:-|:-|
| `-d` | Yes | will run the container in the background. |
| `--name (container name)` | No | Sets the name of the container to the following word. You can change this to whatever you want. |
| `-p (ports)` | Yes | Depending on how your network is setup, you may need to portforward 80, 443, and the management port. |
| `-v (path to storage directory):/opt/zoraxy/config/` | Recommend | Sets the folder that holds your files. This should be the place you just chose. By default, it will create a Docker volume for the files for persistency but they will not be accessible. |
| `-e ARGS='(your arguments)'` | No | Sets the arguments to run Zoraxy with. Enter them as you would normally. By default, it is ran with `-noauth=false` but <b>you cannot change the management port.</b> This is required for the healthcheck to work. |
| `zoraxydocker/zoraxy:latest` | Yes | The repository on Docker hub. By default, it is the latest version that is published. |

## Examples: </br>
### Docker Run </br>
```
docker run -d --name zoraxy -p 80:80 -p 443:443 -p 8005:8000/tcp -v /home/docker/Containers/Zoraxy:/opt/zoraxy/config/ -e ARGS='-noauth=false' zoraxydocker/zoraxy:latest
```

### Docker Compose </br>
```yml
version: '3.3'
services:
  zoraxy-docker:
    image: zoraxydocker/zoraxy:latest
    container_name: zoraxy
    ports:
      - 80:80
      - 443:443
      - 8005:8000/tcp
    volumes:
      - /home/docker/Containers/Zoraxy:/opt/zoraxy/config/
    environment:
      ARGS: '-noauth=false'
```
