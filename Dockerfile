FROM golang:1.14.2

LABEL homepage="https://tarkov-database.com"
LABEL repository="https://github.com/tarkov-database/tileserver"
LABEL maintainer="Markus Wiegand <mail@morphy2k.dev>"

EXPOSE 8080

WORKDIR /tmp/github.com/tarkov-database/tileserver
COPY . .

RUN make bin && \
    mkdir -p /usr/share/tarkov-database/tileserver && \
    mv -t /usr/share/tarkov-database/tileserver tileserver && \
    rm -rf /tmp/github.com/tarkov-database/tileserver

WORKDIR /usr/share/tarkov-database/tileserver

CMD ["/usr/share/tarkov-database/tileserver/tileserver"]
