---
layout: home

hero:
  name: GitHub Actions
  text: Cache Server
  tagline: A small, fast, self-hosted GitHub Actions cache. Drop-in compatible with actions/cache@v4 and @v5. Single static binary.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: How it works
      link: /how-it-works

features:
  - title: One static binary
    details: ~12MB, no runtime, no dependencies. Distroless image.
    icon: 🪶
  - title: Storage drivers
    details: filesystem, S3 / MinIO, Google Cloud Storage.
    icon: 📦
  - title: Database drivers
    details: SQLite (default), PostgreSQL, MySQL.
    icon: 🗄️
  - title: No workflow changes
    details: Uses the same cache protocol — point your runner at the server and you're done.
    icon: ⚙️
  - title: Helm chart
    details: First-class Kubernetes deploy. Auto PVC for filesystem/sqlite, HPA-aware.
    icon: ⛵
  - title: Management API
    details: REST + OpenAPI/Swagger UI for inspecting and pruning cache.
    icon: 🔧
---
