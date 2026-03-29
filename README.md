<p align="center">
  <img src="https://readme-typing-svg.demolab.com?font=Fira+Code&weight=700&size=28&duration=3000&pause=1000&color=3898EC&center=true&vCenter=true&multiline=true&repeat=true&width=500&height=80&lines=%E2%9B%A9%EF%B8%8F+MLflow+Portal+%E2%9B%A9%EF%B8%8F;Authentication+Gateway+%26+Admin+Panel" alt="Typing SVG" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/STATUS-PRODUCTION_READY-00ff88?style=for-the-badge&labelColor=0a0e1a" />
  <img src="https://img.shields.io/badge/STACK-FULL_STACK-3898ec?style=for-the-badge&labelColor=0a0e1a" />
</p>

<p align="center">
  <b>Custom authentication gateway & admin panel for MLflow</b><br/>
  <sub>Go + Svelte 5 + Docker Compose — no external MLflow port, full permission control</sub>
</p>

<p align="center">
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/Svelte-5_(Runes)-FF3E00?style=flat-square&logo=svelte&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/MLflow-3.10-0194E2?style=flat-square&logo=mlflow&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/PostgreSQL-Alpine-4169E1?style=flat-square&logo=postgresql&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/MinIO-S3-C72E49?style=flat-square&logo=minio&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/JWT-HS256-000000?style=flat-square&logo=jsonwebtokens&logoColor=white" /></a>
  <a href="#-tech-stack"><img src="https://img.shields.io/badge/Vite-Build-646CFF?style=flat-square&logo=vite&logoColor=white" /></a>
</p>

<p align="center">
  <img src="https://capsule-render.vercel.app/api?type=waving&color=gradient&customColorList=2,3,12,14&height=120&section=header&animation=twinkling" width="100%" />
</p>

---

## ✦ About

**MLflow Portal** คือ custom authentication & admin gateway ที่ทำหน้าที่เป็นประตูทางเข้าไปยัง MLflow server — แทนที่จะเปิด MLflow ตรงๆ Portal จะจัดการ authentication, reverse proxy, และ permission control ทั้งหมดให้

- **JWT Authentication** — login ผ่าน MLflow API แล้วได้รับ session cookie (24h expiry)
- **Reverse Proxy** — proxy ไปยัง MLflow UI พร้อม inject Basic Auth header อัตโนมัติ
- **Admin Panel** — จัดการ users, experiment permissions, model permissions ผ่าน UI
- **UI Injection** — inject floating bar เข้าไปใน MLflow UI เพื่อ logout / กลับ admin ได้เลย

---

## ⚡ Tech Stack

| Layer | Technology | Role |
|:---:|:---|:---|
| ⚔️ **Gateway** | **Go 1.23** | Reverse proxy, JWT auth, admin API, session management |
| 🎴 **Frontend** | **Svelte 5 (Runes)** + Vite | Login page, admin dashboard (users, permissions) |
| 🧪 **ML Platform** | **MLflow 3.10** | Experiment tracking, model registry, artifact storage |
| 🗄️ **Database** | **PostgreSQL (Alpine)** | MLflow backend store + auth user database |
| 📦 **Artifacts** | **MinIO** (S3-compatible) | Model artifacts & experiment data storage |
| 🐳 **Container** | **Docker Compose** | Multi-stage builds, health checks, resource limits |
| 🔐 **Auth** | **JWT (HS256)** + Basic Auth proxy | Stateless sessions, cookie-based, 24h expiry |

---

## ◈ Architecture

```
                    ┌─────────────────────────────────────────────┐
                    │           Docker Compose Network             │
   User Request     │                                             │
   ─────────────►   │  ┌──────────┐    reverse     ┌───────────┐ │
        :80         │  │  Portal   │───proxy──────►│  MLflow    │ │
                    │  │  (Go)     │   /mlflow/*    │  Server    │ │
                    │  │  :8080    │                │  :5000     │ │
                    │  └──────────┘                └─────┬──────┘ │
                    │       │                            │    │    │
                    │       │ JWT + Basic Auth      ┌────┘    │    │
                    │       │                       │         │    │
                    │  ┌────▼─────┐          ┌──────▼──┐  ┌──▼──┐ │
                    │  │ Svelte 5 │          │ Postgres │  │MinIO│ │
                    │  │ Frontend │          │   :5432  │  │:9000│ │
                    │  │ (static) │          └─────────┘  └─────┘ │
                    │  └──────────┘                                │
                    └─────────────────────────────────────────────┘
```

---

## ⬡ Project Structure

```
mlflow-portal/
├── docker-compose.yml        # Orchestration — ทุกอย่างเริ่มจากที่นี่
├── .env                      # Secrets & config (you create this)
├── Dockerfile                # MLflow server image
├── basic_auth.ini            # MLflow auth config
│
└── portal/                   # The Gateway
    ├── Dockerfile            # Multi-stage: Node → Go → Alpine
    ├── main.go               # Go backend (proxy + API + auth)
    ├── go.mod / go.sum       # Go dependencies
    │
    └── frontend/             # Svelte 5 SPA
        ├── index.html
        ├── svelte.config.js
        └── src/
            ├── main.js
            ├── app.css       # Dark cyberpunk theme
            └── App.svelte    # Single-file app component
```

---

## 🚀 Quick Start

### 1. สร้างไฟล์ `.env`

```env
# ─── PostgreSQL ───
POSTGRES_VERSION=16
DB_USER=mlflow
DB_PASSWORD=Connected@2022
DB_NAME=mlflow
DB_PORT=5432

# ─── MinIO (S3) ───
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin123
MINIO_VERSION=latest
MINIO_MC_VERSION=latest
MLFLOW_S3_BUCKET=mlflow

# ─── MLflow ───
MLFLOW_VERSION=3.10.1
MLFLOW_FLASK_SERVER_SECRET_KEY=your-super-secret-key-change-this
```

### 2. Start 🔥

```bash
docker compose up -d --build
```

### 3. เข้าใช้งาน

| URL | Description |
|:---|:---|
| `http://localhost` | Portal login → admin panel หรือ MLflow UI |
| `http://localhost/mlflow/` | MLflow UI (ต้อง login ก่อน) |
| `http://localhost:9001` | MinIO Console |

> **Default admin** → `admin` / `Connected@2022`

---

## ◎ Services

| Service | Container | Port | Memory |
|:---|:---|:---:|:---:|
| PostgreSQL | `mlflow_db` | internal | 1 GB |
| MinIO | `mlflow_minio` | 9001 (console) | 512 MB |
| MLflow Server | `mlflow_server` | internal | 2 GB |
| Portal (Go+Svelte) | `mlflow_portal` | **80** | 128 MB |

---

## ⟡ API Endpoints

### 🔓 Auth

| Method | Path | Description |
|:---:|:---|:---|
| `POST` | `/api/login` | Login → JWT cookie |
| `POST` | `/api/logout` | Clear session |
| `GET` | `/api/me` | Current user info |

### 👥 User Management *(admin only)*

| Method | Path | Description |
|:---:|:---|:---|
| `GET` | `/api/admin/users` | List all users |
| `GET` | `/api/admin/users/get?username=` | Get user detail |
| `POST` | `/api/admin/users/create` | Create user |
| `DELETE` | `/api/admin/users/delete` | Delete user |
| `PATCH` | `/api/admin/users/update-password` | Reset password |
| `PATCH` | `/api/admin/users/update-admin` | Toggle admin role |

### 🔑 Permissions *(experiment & model)*

| Method | Path | Description |
|:---:|:---|:---|
| `POST` | `/api/admin/experiment-permissions/create` | Grant experiment perm |
| `PATCH` | `/api/admin/experiment-permissions/update` | Update experiment perm |
| `DELETE` | `/api/admin/experiment-permissions/delete` | Revoke experiment perm |
| `POST` | `/api/admin/model-permissions/create` | Grant model perm |
| `PATCH` | `/api/admin/model-permissions/update` | Update model perm |
| `DELETE` | `/api/admin/model-permissions/delete` | Revoke model perm |

### 📋 Resources

| Method | Path | Description |
|:---:|:---|:---|
| `GET` | `/api/admin/experiments` | List all experiments |
| `GET` | `/api/admin/models` | List registered models |

---

## ↻ How It Works

### Login Flow

```
User ──► POST /api/login ──► Go Portal ──► validate ──► MLflow API
                                  │
                                  ▼
                        JWT cookie (mlflow_session)
                      + creds cookie (mlflow_creds)
                                  │
                                  ▼
                    admin? ──► /admin panel
                    user?  ──► /mlflow/ (redirect)
```

### Proxy Flow

```
User ──► GET /mlflow/* ──► Portal (check JWT)
                              │
                              ▼
                    inject Basic Auth header
                              │
                              ▼
                    forward to MLflow :5000
                              │
                              ▼
                    rewrite HTML paths (/static-files/ → /mlflow/static-files/)
                    inject floating Logout/Admin bar
                              │
                              ▼
                         ◄── User
```

---

## 🛡️ Security Notes

- **MLflow ไม่มี external port** — เข้าถึงได้ผ่าน Portal proxy เท่านั้น
- **Password cookies** — base64 encoded, HttpOnly, SameSite=Lax → **ใช้ HTTPS ใน production!**
- **JWT secret** — ควร generate ด้วย `openssl rand -hex 32`
- **Default permission** = `NO_PERMISSIONS` → ต้อง grant สิทธิ์ให้ user ทุกคน

---

## 🎨 UI Theme

Frontend ใช้ **dark cyberpunk theme** พร้อม glassmorphism card effects, blue-cyan gradient accents (`#3898ec` → `#1ce5d6`), animated particle background บนหน้า login, Inter font family และ responsive design ทั้งระบบ

---

<p align="center">
  <img src="https://capsule-render.vercel.app/api?type=waving&color=gradient&customColorList=2,3,12,14&height=100&section=footer&animation=twinkling" width="100%" />
</p>

<p align="center">
  <sub>
    Built with 💙 and mass amounts of ☕<br/>
    <b>MLflow Portal</b> — Authentication Gateway & Admin Panel
  </sub>
</p>
