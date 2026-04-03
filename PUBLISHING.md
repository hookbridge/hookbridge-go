# Publishing the Go SDK

## Prerequisites

1. Use a clean local checkout with push access to `hookbridge/hookbridge-go`.
2. You do not publish Go to a package registry here. The SDK is released by pushing a git tag.

## Publishing Steps

### 1. Update Version Metadata

Update the user-agent/version string in:
- `client.go`

Example:

```go
userAgent = "hookbridge-go/1.3.1"
```

### 2. Run Verification

```bash
go test ./...
```

### 3. Create and Push Tag

```bash
git tag v1.3.1
git push origin main --tags
```

Typical release order:

1. bump the version string in `client.go`
2. run `go test ./...`
3. commit the release changes
4. create the release tag locally
5. push the commit and tag to GitHub
6. optionally create a GitHub release page

## Notes

The Go SDK is consumed from Git tags on `github.com/hookbridge/hookbridge-go`, so publishing is creating and pushing the release tag after tests pass.
