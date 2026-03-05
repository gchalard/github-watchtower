group "default" {
    targets = [
        "__default__"
    ]
}


variable "registry" {
  default = "ghcr.io"
}

variable "image" {
  default = "gchalard/github-watchtower"
}

variable "tag" {
  default = "latest"
}

variable "cache" {
  default = [{
    type = "registry"
    ref = "${registry}/${image}:cache"
  }]
}

target "__default__" {
  dockerfile = "Dockerfile"
  context = "."
  tags = [ "${registry}/${image}:${tag}" ]
  platforms = [ "linux/amd64", "linux/arm64" ]
}