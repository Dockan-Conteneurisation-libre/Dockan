# Python Example With Dockan

This example runs a small local Python application.

## Files

```text
myapp/
  Dockanfile
  app.py
```

## app.py

```python
print("Hello from Python in Dockan")
```

## Dockanfile

```dockerfile
FROM python:3.12
LABEL org.opencontainers.image.title=PythonExample
WORKDIR /app
COPY app.py ./app.py
CMD python3 app.py
```

Dockan does not download from Docker Hub. If no `python:3.12` Dockan base exists, Dockan uses the local `python3` command.

Import a local base:

```bash
dockan base import python:local ./python-rootfs
```

## Build And Run

```bash
dockan build -t python-demo:latest .
dockan run python-demo:latest
```

## Version Without A Python Base

If Python is already available on the machine and you only want a quick test:

```dockerfile
FROM scratch
LABEL org.opencontainers.image.title=PythonHostDemo
COPY app.py /app.py
CMD python3 app.py
```

This version depends on the local environment, so it is not as portable as a real `python:local` base.
