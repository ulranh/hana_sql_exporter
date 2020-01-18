# build stage
FROM golang as builder

WORKDIR /app
# ENV GO111MODULE=on

COPY go.mod .
COPY go.sum .

RUN go mod download

# FROM build_base AS server_builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build


# final stage
FROM scratch
COPY --from=builder /app/hana_sql_exporter /app/

EXPOSE 3030
ENTRYPOINT ["/app/hana_sql_exporter","web","--port","3030","--config","/app/hana_sql_exporter.toml"]
