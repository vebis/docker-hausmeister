FROM golang:latest as builder
MAINTAINER Stephan Kirsten <vebis@gmx.net>
LABEL description="docker-hausmeister builder container"
WORKDIR /src/
COPY ./assets/build/app.go .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM docker:latest
MAINTAINER Stephan Kirsten <vebis@gmx.net>
LABEL description="docker-hausmeister docker container"
WORKDIR /root/
COPY --from=builder /src/app .
CMD [ "./app" ]
