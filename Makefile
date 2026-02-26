PREFIX ?= /usr
BINDIR ?= ${PREFIX}/bin

.PHONY: install
install: serveroute
	install -m 0755 ./serveroute ${BINDIR}/serveroute

.PHONY: serveroute
serveroute:
	go build .
