FROM docker.io/library/golang:1.23.3 as builder

RUN mkdir -p /build

WORKDIR /build

COPY go.mod .

RUN go mod download

COPY main.go .

RUN go get

RUN go vet -v

RUN go build --ldflags '-extldflags "-lm"'

RUN mv /build/bws-adapter /usr/bin/

CMD ["/usr/bin/bws-adapter"]
