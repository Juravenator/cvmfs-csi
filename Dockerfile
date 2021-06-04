FROM golang:1.16.4-alpine as builder

WORKDIR /workdir
COPY . .
RUN apk add make bash git && CGO_ENABLED=0 make build

FROM centos:7
LABEL description="CernVM-FS CSI Plugin"

RUN yum install -y http://ecsft.cern.ch/dist/cvmfs/cvmfs-release/cvmfs-release-latest.noarch.rpm && \
    yum install -y cvmfs && yum clean all && rm -rf /var/cache/yum

COPY --from=builder /workdir/bin/csi-cvmfsplugin /csi-cvmfsplugin
RUN chmod +x /csi-cvmfsplugin
ENTRYPOINT ["/csi-cvmfsplugin"]
