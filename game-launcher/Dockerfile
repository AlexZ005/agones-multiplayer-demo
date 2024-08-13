FROM golang:1.21

WORKDIR /usr/src/app

COPY web/ /usr/src/app/web
# COPY .env /usr/src/app/.env
COPY ./certs /usr/src/app/certs
COPY ./main /usr/local/bin/app
COPY config /root/.kube/config

CMD ["app"]