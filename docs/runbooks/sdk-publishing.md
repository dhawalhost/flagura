# 📦 SDK Release & Publishing Runbook

This guide covers the end-to-end procedures for building, testing, versioning, and publishing Flagura's official client SDKs across public package registries (**Go Modules**, **NPM**, **PyPI**, **Crates.io**, and the **OpenFeature Ecosystem**).

---

## 🧭 Multi-Language SDK Map

| Language / Platform | Repository Directory | Public Registry | Package Name | Installation Command |
| :--- | :--- | :--- | :--- | :--- |
| **Go** (Native + OpenFeature) | [`sdks/go`](../../sdks/go) | Go Modules (`proxy.golang.org`) | `github.com/dhawalhost/flagura/sdks/go` | `go get github.com/dhawalhost/flagura/sdks/go` |
| **TypeScript / JavaScript / React** | [`sdks/js`](../../sdks/js) | NPM (`npmjs.com`) | `@flagura/sdk` | `npm install @flagura/sdk` |
| **Python** (Native + OpenFeature) | [`sdks/python`](../../sdks/python) | PyPI (`pypi.org`) | `flagura-sdk` | `pip install flagura-sdk` |
| **Rust** | [`sdks/rust`](../../sdks/rust) | Crates.io (`crates.io`) | `flagura` | `cargo add flagura` |

---

## 1. Go SDK & OpenFeature Provider (`sdks/go`)

The Go SDK is structured as an isolated, dedicated submodule (`github.com/dhawalhost/flagura/sdks/go`) with **zero server dependencies** (`lib/pq`, `templ`, etc. are excluded).

### Release Procedures:

1. **Run Local Validation**:
   ```bash
   cd sdks/go
   go test -v -race ./...
   ```
2. **Create Submodule Git Tag**:
   In Go multi-module repositories, submodule tags **must** be prefixed with the submodule directory path:
   ```bash
   # Format: sdks/go/v<SemVer>
   git tag sdks/go/v1.0.0
   git push origin sdks/go/v1.0.0
   ```
3. **Trigger Go Proxy & pkg.go.dev Indexing**:
   Force `proxy.golang.org` to immediately fetch and index the release:
   ```bash
   GOPROXY=https://proxy.golang.org GO111MODULE=on go get github.com/dhawalhost/flagura/sdks/go@v1.0.0
   ```
4. **Verify Installation**:
   ```bash
   # Native Client
   go get github.com/dhawalhost/flagura/sdks/go@v1.0.0

   # OpenFeature Provider
   go get github.com/dhawalhost/flagura/sdks/go/openfeature@v1.0.0
   ```

---

## 2. JavaScript / TypeScript / React SDK (`sdks/js`)

Published to the official [NPM Registry](https://www.npmjs.com) under the `@flagura` organization scope.

### Release Procedures:

1. **Authenticate with NPM**:
   ```bash
   npm login
   ```
2. **Update Version in `package.json`**:
   ```json
   {
     "name": "@flagura/sdk",
     "version": "1.0.0"
   }
   ```
3. **Build Bundle and Type Definitions**:
   ```bash
   cd sdks/js
   npm install
   npm run build
   ```
   *(Generates ESM, CJS, and TypeScript `.d.ts` declaration maps in `dist/`)*
4. **Publish to NPM**:
   ```bash
   npm publish --access public
   ```
5. **Verify Installation**:
   ```bash
   npm install @flagura/sdk@latest
   ```

---

## 3. Python SDK (`sdks/python`)

Published to [PyPI](https://pypi.org) using `pyproject.toml`, `build`, and `twine`.

### Release Procedures:

1. **Install Build Tools**:
   ```bash
   pip install --upgrade build twine
   ```
2. **Update Version in `pyproject.toml`**:
   ```toml
   [project]
   name = "flagura-sdk"
   version = "1.0.0"
   ```
3. **Clean and Build Distributions**:
   ```bash
   cd sdks/python
   rm -rf dist/ build/ *.egg-info
   python3 -m build
   ```
4. **Validate Package Distribution**:
   ```bash
   twine check dist/*
   ```
5. **Upload to PyPI**:
   ```bash
   # Upload to TestPyPI first (optional):
   twine upload --repository testpypi dist/*

   # Upload to Production PyPI:
   twine upload dist/*
   ```
   *(Enter your PyPI API Token `pypi-...` when prompted)*
6. **Verify Installation**:
   ```bash
   pip install flagura-sdk==1.0.0
   ```

---

## 4. Rust SDK (`sdks/rust`)

Published to [crates.io](https://crates.io) via `cargo`.

### Release Procedures:

1. **Log in to Crates.io**:
   ```bash
   cargo login <your_crates_io_api_token>
   ```
2. **Update Version in `Cargo.toml`**:
   ```toml
   [package]
   name = "flagura"
   version = "1.0.0"
   ```
3. **Run Local Tests & Dry Run**:
   ```bash
   cd sdks/rust
   cargo test
   cargo publish --dry-run
   ```
4. **Publish to Crates.io**:
   ```bash
   cargo publish
   ```
5. **Verify Installation**:
   ```bash
   cargo add flagura
   ```

---

## 5. OpenFeature Ecosystem Catalog Listing

To submit Flagura as an official certified provider in the vendor-neutral [OpenFeature Ecosystem Catalog](https://github.com/open-feature/ecosystem):

1. **Fork the Ecosystem Repository**:
   Fork [open-feature/ecosystem](https://github.com/open-feature/ecosystem) on GitHub.
2. **Add Provider Entry**:
   Create `ecosystem/providers/flagura.md`:
   ```markdown
   # Flagura Provider

   - **Supported Languages**: Go, TypeScript/JavaScript, Python
   - **Repository**: [github.com/dhawalhost/flagura](https://github.com/dhawalhost/flagura)
   - **Go Package**: `github.com/dhawalhost/flagura/sdks/go/openfeature`
   - **NPM Package**: `@flagura/sdk`
   - **PyPI Package**: `flagura-sdk`
   ```
3. **Submit Pull Request**:
   Open a PR against `open-feature/ecosystem` main branch for review and inclusion in the official OpenFeature directory.

---

## 📋 Release Checklist Template

```markdown
- [ ] 1. All unit and race detector tests passing locally (`go test -race ./...`, `cargo test`, `npm test`, `pytest`).
- [ ] 2. Update version numbers across `sdks/*/` config files.
- [ ] 3. Tag Go submodule: `git tag sdks/go/vX.Y.Z && git push origin sdks/go/vX.Y.Z`.
- [ ] 4. Publish NPM: `cd sdks/js && npm run build && npm publish --access public`.
- [ ] 5. Publish PyPI: `cd sdks/python && python3 -m build && twine upload dist/*`.
- [ ] 6. Publish Crates: `cd sdks/rust && cargo publish`.
- [ ] 7. Verify index status on `pkg.go.dev`, `npmjs.com`, `pypi.org`, and `crates.io`.
```
