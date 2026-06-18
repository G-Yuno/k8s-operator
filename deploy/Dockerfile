FROM golang:1.25.7 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN #go mod download
RUN go env -w GOPROXY=https://goproxy.cn,direct ; go mod download -x
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal


RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details


#FROM cytopia/ansible as runner
FROM registry.cn-beijing.aliyuncs.com/ning1875_haiwai_image/busybox:ansible as runner

#FROM ansible/ansible as runner
WORKDIR /
COPY --from=builder /workspace/manager .
ENTRYPOINT ["/manager"]