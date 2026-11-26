# use PowerShell instead of sh on Windows:
set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

set quiet := true

# Run tests (short)
[default]
test:
  echo "Running tests"
  go test --short ./...

[windows]
test_pdf:
    go build ./cmd/resume
    ./resume.exe build resumes/example.yml --output test.pdf