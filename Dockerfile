FROM moss-platform-cn-shanghai.cr.volces.com/mosi-matrix/golang:1.26.1-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY workflow/api-server ./workflow/api-server
COPY packages/backend/go ./packages/backend/go
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd workflow/api-server && \
    CGO_ENABLED=0 GOOS=linux GOWORK=off go build -trimpath -o /out/workflow ./cmd/workflow && \
    CGO_ENABLED=0 GOOS=linux GOWORK=off go build -trimpath -o /out/worker ./cmd/worker

FROM moss-platform-cn-shanghai.cr.volces.com/mosi-matrix/alpine:3.23.3
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/workflow /app/workflow
COPY --from=build /out/worker /app/worker
WORKDIR /app
EXPOSE 8080
VOLUME ["/data/conf"]
USER nobody:nobody
CMD ["/app/workflow", "-conf", "/data/conf"]
