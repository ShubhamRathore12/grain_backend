# Server Docker & Nginx Configuration Report

**Server:** `91.98.235.142`  
**Date:** May 30, 2026

---

## Docker Disk Usage Summary

| Type | Total | Active | Size | Reclaimable |
|------|-------|--------|------|-------------|
| Images | 28 | 22 | 5.39 GB | 5.37 GB (99%) |
| Containers | 23 | 11 | 839.7 MB | 839.6 MB (99%) |
| Local Volumes | 4 | 2 | 73.17 MB | 0 B (0%) |
| Build Cache | 130 | 0 | 4.57 GB | 4.33 GB |

**Total reclaimable space: ~10 GB**

---

## Running Containers (Active)

| Container Name | Image | Ports | Status |
|----------------|-------|-------|--------|
| grain-backend | grain_backend-grain-backend | 3000→3000 | Up 14h (healthy) |
| crm-frontend | crm-frontend:latest | 3005→3000 | Up 17h (unhealthy) |
| crm-api | crm-backend-api | 4200→8080 | Up 24h (healthy) |
| crm-supabase-gateway | nginx:alpine | 80 (internal) | Up 24h (unhealthy) |
| crm-postgrest | postgrest/postgrest:v12.0.2 | 3400→3000 | Up 24h |
| crm-campaign-worker | crm-backend-campaign-worker | 3000 (internal) | Up 24h (unhealthy) |
| crm-postgres | postgres:15-alpine | 5433→5432 | Up 24h (healthy) |
| crm-redis | redis:7.2-alpine | 6380→6379 | Up 24h (healthy) |
| machine-config-service | machine-config-machine-config | 8080→8080 | Up 3d (healthy) |
| myshaa-phpmyadmin | phpmyadmin:5-apache | 127.0.0.1:8082→80 | Up 13d |
| myshaa-mysql57 | mysql:5.7 | 3306→3306 | Up 13d |

---

## Stopped/Exited Containers (Dead Weight)

| Container Name | Image | Exited | Age |
|----------------|-------|--------|-----|
| wizardly_clarke | f883664565cd (untagged) | Exit 1 | 17h ago |
| gallant_mclean | 46d88f20f820 (untagged) | Exit 1 | 18h ago |
| angry_panini | 9b84c4358da9 (untagged) | Exit 1 | 18h ago |
| keen_elbakyan | 4bca7226d4b0 (untagged) | Exit 1 | 18h ago |
| focused_shaw | 9b590f779cee (untagged) | Exit 1 | 18h ago |
| wizardly_volhard | 0b93f1816336 (untagged) | Exit 1 | 18h ago |
| unruffled_bohr | 69b989097e9f (untagged) | Exit 1 | 18-19h ago |
| thirsty_gagarin | 7bb0d964e43e (untagged) | Exit 1 | 24h ago |
| sharp_nobel | eb5efa3661f4 (untagged) | Exit 1 | 2d ago |
| jolly_cannon | 5ec432abed0a (untagged) | Exit 1 | 2d ago |
| gracious_hypatia | 79f8c199bd94 (untagged) | Exit 1 | 2d ago |
| myshaa-phpmyadmin-utc-backup-20260429 | phpmyadmin:5-apache | Exit 0 | 4w ago |

---

## Dangling (Untagged) Images

| Image ID | Size | Notes |
|----------|------|-------|
| d873e5f27633 | 21.1 MB | Old grain_backend build |
| b71b9e1465c9 | 21.1 MB | Old grain_backend build |
| 9c06a0b8c14a | 21.1 MB | Old grain_backend build |
| f883664565cd | 775 MB | Failed CRM frontend build |
| 46d88f20f820 | 774 MB | Failed CRM frontend build |
| 9b84c4358da9 | 1.02 GB | Failed CRM frontend build |
| 4bca7226d4b0 | 1.02 GB | Failed CRM frontend build |
| 9b590f779cee | 1.02 GB | Failed CRM frontend build |
| 0b93f1816336 | 1.02 GB | Failed CRM frontend build |
| 69b989097e9f | 1.02 GB | Failed CRM frontend build |
| 7bb0d964e43e | 1.02 GB | Failed CRM frontend build |
| eb5efa3661f4 | 842 MB | Failed CRM frontend build |
| 5ec432abed0a | 796 MB | Failed CRM frontend build |
| 79f8c199bd94 | 775 MB | Failed CRM frontend build |

**Total dangling images: ~10.5 GB of wasted space**

---

## Tagged Docker Images

| Image | Size | In Use? |
|-------|------|---------|
| crm-backend-api:latest | 271 MB | ✅ Yes |
| crm-backend-campaign-worker:latest | 271 MB | ✅ Yes |
| crm-backend-optimized-api:latest | 271 MB | ❌ No (duplicate/old) |
| crm-backend-optimized-campaign-worker:latest | 271 MB | ❌ No (duplicate/old) |
| crm-frontend:latest | 216 MB | ✅ Yes |
| grain_backend-grain-backend:latest | 21.1 MB | ✅ Yes |
| machine-config-machine-config:latest | 15.3 MB | ✅ Yes |
| mysql:5.7 | 501 MB | ✅ Yes |
| nginx:alpine | 62.3 MB | ✅ Yes |
| node:20-alpine | 136 MB | ❌ No (build cache only) |
| phpmyadmin:5-apache | 575 MB | ✅ Yes |
| postgres:15-alpine | 274 MB | ✅ Yes |
| postgrest/postgrest:v12.0.2 | 17.4 MB | ✅ Yes |
| redis:7.2-alpine | 38.6 MB | ✅ Yes |

---

## Docker Volumes

| Volume | In Use? |
|--------|---------|
| crm-backend_postgres_data | ✅ Yes (CRM PostgreSQL) |
| crm-backend_redis_data | ✅ Yes (CRM Redis) |
| crm-backend-optimized_postgres_data | ❌ No (old/duplicate) |
| crm-backend-optimized_redis_data | ❌ No (old/duplicate) |

---

## Docker Networks

| Network | Driver | In Use? |
|---------|--------|---------|
| bridge | bridge | ✅ Default |
| crm-backend_crm-network | bridge | ✅ Yes |
| crm-frontend_crm-network | bridge | ✅ Yes |
| crm-backend-optimized_crm-network | bridge | ❌ Likely unused |
| crm_frontend_nextjs_crm-network | bridge | ❌ Likely unused |
| grain_backend_grain-net | bridge | ✅ Yes |
| machine-config_default | bridge | ✅ Yes |
| myshaa-dbnet | bridge | ✅ Yes |

---

## Docker Compose Projects

### 1. Grain Backend (`/opt/grain_backend/docker-compose.yml`)
```yaml
services:
  grain-backend:
    build: . (Dockerfile)
    container_name: grain-backend
    ports: "3000:3000"
    restart: always
    healthcheck: /api/health
    network: grain-net
```

### 2. Machine Config (`/opt/machine-config/docker-compose.yml`)
```yaml
services:
  machine-config:
    build: .
    container_name: machine-config-service
    ports: "8080:8080"
    restart: unless-stopped
    healthcheck: /api/health
```

### 3. CRM Backend (`/opt/crm-backend/docker-compose.yml`)
```yaml
services:
  postgres:     (postgres:15-alpine, port 5433)
  postgrest:    (postgrest/postgrest:v12.0.2, port 3400)
  supabase-gateway: (nginx:alpine, internal)
  redis:        (redis:7.2-alpine, port 6380)
  api:          (built from Dockerfile, port 4200)
  email-worker: (built from Dockerfile, internal)
  campaign-worker: (built from Dockerfile, internal)
```

### 4. CRM Frontend (`/opt/crm_frontend_nextjs/docker-compose.yml`)
```yaml
services:
  crm-frontend:
    build: . (Dockerfile)
    image: crm-frontend:latest
    container_name: crm-frontend
    ports: "3005:3000"
    restart: unless-stopped
```

### 5. CRM Backend Optimized (`/opt/crm-backend-optimized/docker-compose.yml`)
- **Duplicate** of `/opt/crm-backend/docker-compose.yml` (same config)
- Not actively running separate containers (shares same container names)

---

## Nginx Configuration

### Main Config (`/etc/nginx/nginx.conf`)
- Standard Ubuntu/Debian nginx config
- Includes `sites-enabled/*` and `conf.d/*.conf`
- Gzip enabled, SSL configured

### Sites Enabled (Active)

| File | Purpose |
|------|---------|
| `crm` | CRM frontend proxy at `/crm` → port 3005 |
| `primeosys.com` | Main site + grain backend at `/backend/` → port 3000 |

### Sites Available (All Configs)

#### `primeosys.com` (ENABLED)
- Domain: `primeosys.com`, `www.primeosys.com`
- SSL: Let's Encrypt
- Root: `/var/www/primeosys.com`
- `/backend/` → `127.0.0.1:3000` (Grain Backend)
- `/machine-config/api/` → `127.0.0.1:8080` (Machine Config, via snippet)
- HTTP → HTTPS redirect

#### `crm` (ENABLED)
- Domain: `primeosys.com`, `www.primeosys.com`, `91.98.235.142`
- `/crm` → `127.0.0.1:3005` (CRM Frontend)
- HTTP only (port 80)

#### `indiamart` (NOT ENABLED)
- Listens on port 80 (default_server) and 443 (default_server)
- Domain: `91.98.235.142`
- Root: `/var/www/primeosys.com`
- `/api/` → `127.0.0.1:3000` (Grain Backend)
- `/ws` → `127.0.0.1:3000` (WebSocket)
- `/webhook` → `127.0.0.1:9000` (GitHub webhook)
- SSL uses leadops cert

#### `grain-api` (NOT ENABLED)
- Domain: `91.98.235.142`
- `/` → `127.0.0.1:3000` (Grain Backend)
- HTTP only

#### `myshaa.com` (NOT ENABLED)
- Domain: `myshaa.com`, `www.myshaa.com`
- SSL: Let's Encrypt
- Root: `/var/www/myshaa.com/public` (PHP/Laravel app)
- PHP-FPM 8.3
- `/kabu/` routing for viewer

#### `bulkemails.myshaa.com` (NOT ENABLED)
- Domain: `bulkemails.myshaa.com`
- SSL: Let's Encrypt
- Root: `/var/www/bulkemails.myshaa.com` (PHP app)
- PHP-FPM 8.3

#### `leadops.prosafeautomation.com` (NOT ENABLED)
- Domain: `leadops.prosafeautomation.com`
- SSL: Let's Encrypt (Certbot)
- Proxy → `127.0.0.1:5000`

#### `leadsapi.prosafeautomation.com` (NOT ENABLED)
- Domain: `leadsapi.prosafeautomation.com`
- SSL: Let's Encrypt (Certbot)
- Proxy → `127.0.0.1:4000` (with WebSocket support)

#### `myshaa-phpmyadmin-ssl.conf` (NOT ENABLED)
- Listens on port 8081 (SSL)
- Domain: `myshaa.com`
- Proxy → `127.0.0.1:8082` (phpMyAdmin container)

---

## 🗑️ WHAT YOU CAN SAFELY REMOVE

### Containers to Remove (all exited with error)
```bash
# Remove all stopped containers at once
docker container prune -f

# Or remove individually:
docker rm wizardly_clarke gallant_mclean angry_panini keen_elbakyan focused_shaw wizardly_volhard unruffled_bohr thirsty_gagarin sharp_nobel jolly_cannon gracious_hypatia myshaa-phpmyadmin-utc-backup-20260429
```
**Space saved: ~839 MB**

### Images to Remove
```bash
# Remove ALL dangling (untagged) images — these are all failed builds
docker image prune -f

# Remove unused tagged images:
docker rmi crm-backend-optimized-api:latest
docker rmi crm-backend-optimized-campaign-worker:latest
docker rmi node:20-alpine
```
**Space saved: ~11 GB**

### Build Cache to Clear
```bash
docker builder prune -f
```
**Space saved: ~4.3 GB**

### Unused Volumes (⚠️ verify data is not needed)
```bash
# These belong to the "optimized" project that's a duplicate
docker volume rm crm-backend-optimized_postgres_data
docker volume rm crm-backend-optimized_redis_data
```

### Unused Networks
```bash
docker network rm crm-backend-optimized_crm-network
docker network rm crm_frontend_nextjs_crm-network
```

### One-Command Full Cleanup
```bash
# Nuclear option — removes ALL unused containers, images, networks, and build cache
docker system prune -a -f --volumes
```
⚠️ **WARNING:** The nuclear option will also remove `crm-backend-optimized` volumes. Only use if you're sure that data isn't needed.

---

## Recommended Safe Cleanup (Run in Order)

```bash
# Step 1: Remove stopped containers
docker container prune -f

# Step 2: Remove dangling images (failed builds)
docker image prune -f

# Step 3: Remove specific unused tagged images
docker rmi crm-backend-optimized-api:latest crm-backend-optimized-campaign-worker:latest node:20-alpine

# Step 4: Clear build cache
docker builder prune -f

# Step 5: Remove unused networks
docker network prune -f
```

**Estimated total space recovered: ~15+ GB**
