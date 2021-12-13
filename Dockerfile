FROM golang:latest as BUILDER

# build binary
RUN mkdir -p /go/src/gitee.com/lxc-launcher
COPY . /go/src/gitee.com/lxc-launcher
RUN cd /go/src/gitee.com/lxc-launcher && CGO_ENABLED=1 go build -v -o ./launcher main.go

# copy binary config and utils
FROM openeuler/openeuler:21.03
RUN mkdir -p /opt/app/
# overwrite config yaml
COPY --from=BUILDER /go/src/gitee.com/lxc-launcher/launcher /opt/app
WORKDIR /opt/app/
ENTRYPOINT ["/opt/app/launcher"]