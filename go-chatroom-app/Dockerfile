FROM golang:1.21

WORKDIR /usr/src/app

COPY ./certs /usr/src/app/certs
COPY ./main /usr/local/bin/app
COPY ./static/index.html /usr/src/app/static/index.html
COPY config /root/.kube/config

# USER server
CMD ["app"]