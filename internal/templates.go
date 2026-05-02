package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AppTemplateOptions struct {
	Language string
	Dir      string
	Name     string
	Force    bool
}

type appTemplate struct {
	language   string
	base       string
	port       string
	dockanfile string
	files      map[string]string
}

func CreateAppTemplate(opts AppTemplateOptions) error {
	language := strings.ToLower(strings.TrimSpace(opts.Language))
	tpl, ok := appTemplates()[language]
	if !ok {
		return fmt.Errorf("template inconnue: %s. Templates: %s", opts.Language, strings.Join(AppTemplateNames(), ", "))
	}

	dir := opts.Dir
	if dir == "" {
		dir = language + "-app"
	}
	appName := opts.Name
	if appName == "" {
		appName = filepath.Base(filepath.Clean(dir))
		if appName == "." || appName == string(filepath.Separator) {
			appName = language + "-app"
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for name, content := range tpl.files {
		target := filepath.Join(dir, name)
		if !opts.Force {
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("%s existe deja, utilisez --force pour remplacer", target)
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		content = strings.ReplaceAll(content, "{{name}}", appName)
		content = strings.ReplaceAll(content, "{{language}}", tpl.language)
		if err := os.WriteFile(target, []byte(content), 0644); err != nil {
			return err
		}
	}

	fmt.Printf("Created %s Dockan app in %s\n", tpl.language, dir)
	fmt.Printf("Next: cd %s && dockan build -t %s:latest . && dockan run -d --name %s -p 8080:%s %s:latest\n", dir, appName, appName, tpl.port, appName)
	if tpl.base != "scratch" {
		fmt.Printf("Runtime required locally: %s for FROM %s\n", runtimeCommandForTemplate(tpl.language), tpl.base)
	}
	return nil
}

func AppTemplateNames() []string {
	names := make([]string, 0, len(appTemplates()))
	for name := range appTemplates() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func appTemplates() map[string]appTemplate {
	return map[string]appTemplate{
		"python": runtimeTemplate("python", "python:3.12", "8000", "app.py", "python3 app.py", `from http.server import BaseHTTPRequestHandler, HTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"Hello from Python on Dockan\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

HTTPServer(("0.0.0.0", 8000), Handler).serve_forever()
`),
		"node": runtimeTemplate("node", "node:20", "3000", "server.js", "node server.js", `const http = require("http");

http.createServer((req, res) => {
  res.writeHead(200, {"content-type": "text/plain"});
  res.end("Hello from Node on Dockan\n");
}).listen(3000, "0.0.0.0");
`),
		"php": runtimeTemplate("php", "php:8.3", "8000", "public/index.php", "php -S 0.0.0.0:8000 -t public", `<?php
header("Content-Type: text/plain");
echo "Hello from PHP on Dockan\n";
`),
		"ruby": runtimeTemplate("ruby", "ruby:3.3", "4567", "app.rb", "ruby app.rb", `require "webrick"

server = WEBrick::HTTPServer.new(:Port => 4567, :BindAddress => "0.0.0.0")
server.mount_proc("/") do |req, res|
  res["Content-Type"] = "text/plain"
  res.body = "Hello from Ruby on Dockan\n"
end
trap("INT") { server.shutdown }
server.start
`),
		"java": runtimeTemplate("java", "openjdk:21", "8080", "Main.java", "sh -c 'javac Main.java && java Main'", `import com.sun.net.httpserver.HttpServer;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;

public class Main {
  public static void main(String[] args) throws Exception {
    HttpServer server = HttpServer.create(new InetSocketAddress("0.0.0.0", 8080), 0);
    server.createContext("/", exchange -> {
      byte[] body = "Hello from Java on Dockan\n".getBytes(StandardCharsets.UTF_8);
      exchange.getResponseHeaders().add("Content-Type", "text/plain");
      exchange.sendResponseHeaders(200, body.length);
      exchange.getResponseBody().write(body);
      exchange.close();
    });
    server.start();
  }
}
`),
		"go": runtimeTemplate("go", "golang:1.22", "8080", "main.go", "go run main.go", `package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from Go on Dockan")
	})
	http.ListenAndServe("0.0.0.0:8080", nil)
}
`),
		"rust": runtimeTemplate("rust", "rust:1.78", "8080", "main.rs", "sh -c 'rustc main.rs -o app && ./app'", `use std::io::Write;
use std::net::TcpListener;

fn main() {
    let listener = TcpListener::bind("0.0.0.0:8080").unwrap();
    for stream in listener.incoming() {
        let mut stream = stream.unwrap();
        let body = "Hello from Rust on Dockan\n";
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).unwrap();
    }
}
`),
		"shell": runtimeTemplate("shell", "scratch", "8080", "app.sh", "sh app.sh", `#!/usr/bin/env sh
echo "Hello from Shell on Dockan"
`),
		"binary": binaryTemplate(),
		"static": binaryTemplate(),
	}
}

func runtimeTemplate(language, base, port, source, command, sourceContent string) appTemplate {
	files := map[string]string{
		"Dockanfile": fmt.Sprintf("FROM %s\nWORKDIR /app\nCOPY . /app\nEXPOSE %s\nCMD %s\n", base, port, command),
		"dockan.yml": fmt.Sprintf("name: {{name}}\nservices:\n  app:\n    build: .\n    image: {{name}}:latest\n    ports:\n      - 8080:%s\n    restart: always\n", port),
		"README.md":  fmt.Sprintf("# {{name}}\n\nDockan %s app.\n\n```bash\ndockan build -t {{name}}:latest .\ndockan run -d --name {{name}} -p 8080:%s {{name}}:latest\n```\n", language, port),
		source:       sourceContent,
	}
	return appTemplate{language: language, base: base, port: port, files: files}
}

func binaryTemplate() appTemplate {
	files := map[string]string{
		"Dockanfile": "FROM scratch\nCOPY app /app\nEXPOSE 8080\nCMD ./app\n",
		"dockan.yml": "name: {{name}}\nservices:\n  app:\n    build: .\n    image: {{name}}:latest\n    ports:\n      - 8080:8080\n    restart: always\n",
		"README.md":  "# {{name}}\n\nDockan static binary app.\n\nBuild your Linux binary as `app`, then run:\n\n```bash\ndockan build -t {{name}}:latest .\ndockan run -d --name {{name}} -p 8080:8080 {{name}}:latest\n```\n",
	}
	return appTemplate{language: "static-binary", base: "scratch", port: "8080", files: files}
}

func runtimeCommandForTemplate(language string) string {
	switch language {
	case "python":
		return "python3"
	case "node":
		return "node"
	case "php":
		return "php"
	case "ruby":
		return "ruby"
	case "java":
		return "java/javac"
	case "go":
		return "go"
	case "rust":
		return "rustc"
	default:
		return language
	}
}
