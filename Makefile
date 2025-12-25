build:
	go build -o tinygit

install: build
	install -m 0755 tinygit /usr/local/bin/tinygit

clean:
	rm -rf .tinygit
	rm -f tinygit
