# zoraxy-docker </br>

[![Repo](https://img.shields.io/badge/Docker-Repo-007EC6?labelColor-555555&color-007EC6&logo=docker&logoColor=fff&style=flat-square)](https://hub.docker.com/r/passivelemon/zoraxy-docker)
[![Version](https://img.shields.io/docker/v/passivelemon/zoraxy-docker/latest?labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/passivelemon/zoraxy-docker)
[![Size](https://img.shields.io/docker/image-size/passivelemon/zoraxy-docker/latest?sort=semver&labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/passivelemon/zoraxy-docker)
[![Pulls](https://img.shields.io/docker/pulls/passivelemon/zoraxy-docker?labelColor-555555&color-007EC6&style=flat-square)](https://hub.docker.com/r/passivelemon/zoraxy-docker)

Docker container for [Zoraxy](https://github.com/tobychui/zoraxy) </br>

## Setup: </br>
There really isn't much here, just make sure you find a good place to store the files on your host. The container will download everything automatically so this place is used to store the files so they aren't deleted when the container is deleted. </br>

### Using Docker run </br>
```
docker run -d --name (container name) -p (ports) -v (path to storage directory):/zoraxy/data/ -e ARGS=(your arguments) -e VERSION=(version) passivelemon/zoraxy-docker:latest
```

### Using Docker Compose </br>
```
version: '3.3'
services:
  zoraxy-docker:
    image: passivelemon/zoraxy-docker:latest
    container_name: (container name)
    ports:
      - 80:80                                        # Http port
      - 443:443                                      # Https port
      - (wanted port):8000                           # Management portal port
    volumes:
      - (path to storage directory):/zoraxy/data/    # Host directory for Zoraxy file storage
    environment:
      ARGS: '-port=:8000'                            # Telling Zoraxy what port to listen on for the management portal. Changing this means building the image yourself with your own exposed port. Might change in the future.
```

| Operator | Need | Details |
|:-|:-|:-|
| `-d` | Yes | will run the container in the background. |
| `--name (container name)` | No | Sets the name of the container to the following word. You can change this to whatever you want. |
| `-v (path to storage directory):/zoraxy/data/` | Recommend | Sets the folder that holds your files. This should be the place you just chose. By default, it will create a Docker volume for the files for persistency but they will not be accessible. |
| `-e ARGS=(your arguments)` | No | Sets the arguments to run Zoraxy with. By default, it is ran with `-port=:8000 -noauth=false` |
| `-e VERSION=(version)` | No | Sets the version of Zoraxy that the container will download. Must be a supported version found on the Zoraxy Github. Defaults to the latest if not set. |
| `passivelemon/zoraxy-docker:latest` | Yes | The repository on Docker hub. By default, it is the latest version that I have published. |

## Examples: </br>
### Docker Run </br>
```
docker run -d --name zoraxy -p 8000:8000/tcp -v /home/docker/Containers/Zoraxy:/zoraxy/data/ -e ARGS="-port=:8000 -noauth=false" passivelemon/zoraxy-docker:latest
```

### Docker Compose </br>
```
version: '3.3'
services:
  zoraxy-docker:
    image: passivelemon/zoraxy-docker:latest
    container_name: zoraxy
    ports:
      - 80:80
      - 443:443
      - 8000:8000
    volumes:
      - /home/docker/Containers/Zoraxy:/zoraxy/data/
    environment:
      ARGS: '-port=:8000'
```

### Other </br>
If the container doesn't start properly, you might be rate limited from GitHub for some amount of time. You can check this by running `curl -s https://api.github.com/repos/tobychui/zoraxy/releases` on the host. If you are, you will just have to wait for a little while or use a VPN. </br>
