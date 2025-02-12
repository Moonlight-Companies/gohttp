FROM golang:1.22.4 as builder

WORKDIR /project

COPY project/go.mod project/go.sum ./
RUN go mod download

FROM builder

COPY --from=builder /project/ .
COPY project /project
WORKDIR /project

RUN CGO_ENABLED=0 GOOS=linux go build -o app *.go

ENTRYPOINT [ "./app" ]
