FROM golang:1.10
ENV SRC github.com/abraithwaite/jeff
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
WORKDIR /go/src/${SRC}
