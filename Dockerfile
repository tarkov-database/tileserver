FROM golang:1.15.1 as build-env

WORKDIR /tmp/github.com/tarkov-database/tileserver
COPY . .

RUN make bin && \
    mkdir -p /usr/share/tarkov-database/tileserver && \
    mv -t /usr/share/tarkov-database/tileserver tileserver

FROM gcr.io/distroless/base-debian10

LABEL homepage="https://tarkov-database.com"
LABEL repository="https://github.com/tarkov-database/tileserver"
LABEL maintainer="Markus Wiegand <mail@morphy2k.dev>"

COPY --from=build-env /usr/share/tarkov-database/tileserver /

EXPOSE 8080

CMD ["/tileserver"]
