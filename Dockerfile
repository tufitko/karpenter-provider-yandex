FROM --platform=${BUILDPLATFORM} golang:1.24.6-alpine3.21 AS builder
RUN apk update && apk add --no-cache make
ENV GO111MODULE=on
WORKDIR /src

COPY ["go.mod", "go.sum", "/src"]
RUN go mod download && go mod verify

COPY . .
ARG TAG
ARG SHA
RUN make build-all-archs

########################################

FROM --platform=${TARGETARCH} scratch AS karpenter-provider-yandex
LABEL org.opencontainers.image.source="https://github.com/tufitko/karpenter-provider-yandex" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.description="karpenter provider for Yandex Cloud"

COPY --from=gcr.io/distroless/static-debian12:nonroot . .
ARG TARGETARCH
COPY --from=builder /src/bin/karpenter-provider-yandex-${TARGETARCH} /bin/karpenter-provider-yandex

ENTRYPOINT ["/bin/karpenter-provider-yandex"]
