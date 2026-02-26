PREFIX ?= /usr
BINDIR ?= ${PREFIX}/bin

.PHONY: serveroute
serveroute:
	go build .

.PHONY: install
install: serveroute
	install -m 0755 ./serveroute ${BINDIR}/serveroute

