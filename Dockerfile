FROM alpine:3.7

COPY critest /critest

ENTRYPOINT ["/critest"]

