FROM alpine

ENTRYPOINT ["/exposecontroller", "--daemon"]

COPY ./out/exposecontroller-linux-amd64 /exposecontroller
