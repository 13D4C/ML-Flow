# Admin Role & Permission Management UI for MLflow Portal

สร้างหน้า Admin Panel ให้ admin สามารถจัดการ users, roles (admin/non-admin), และ experiment/model permissions ผ่าน UI ที่สวยงามในธีม glassmorphism เดิม

## Proposed Changes

### Go Backend Proxy

#### [MODIFY] [main.go](file:///home/d4c/Documents/Connectech/mlflow/portal/main.go)

เพิ่ม admin-only API proxy endpoints ที่ forward ไปยัง MLflow Auth REST API (ใช้ credentials ของ admin ที่ login อยู่):

**User Management APIs** (`/api/admin/...`):
- `GET /api/admin/users` → proxy ไป MLflow `GET /api/2.0/mlflow/users/get?username=...` (วน loop ดึง users จาก DB ผ่าน search endpoint หรือใช้ undocumented list endpoint)
- `POST /api/admin/users` → `POST /api/2.0/mlflow/users/create`
- `DELETE /api/admin/users` → `DELETE /api/2.0/mlflow/users/delete`
- `PATCH /api/admin/users/password` → `PATCH /api/2.0/mlflow/users/update-password`
- `PATCH /api/admin/users/admin` → `PATCH /api/2.0/mlflow/users/update-admin`

**Experiment Permission APIs**:
- `POST /api/admin/experiment-permissions` → `POST /api/2.0/mlflow/experiments/permissions/create`
- `PATCH /api/admin/experiment-permissions` → `PATCH /api/2.0/mlflow/experiments/permissions/update`
- `DELETE /api/admin/experiment-permissions` → `DELETE /api/2.0/mlflow/experiments/permissions/delete`

**Model Permission APIs**:
- `POST /api/admin/model-permissions` → `POST /api/2.0/mlflow/registered-models/permissions/create`
- `PATCH /api/admin/model-permissions` → `PATCH /api/2.0/mlflow/registered-models/permissions/update`
- `DELETE /api/admin/model-permissions` → `DELETE /api/2.0/mlflow/registered-models/permissions/delete`

**Resource Listing** (ใช้ MLflow Tracking API):
- `GET /api/admin/experiments` → `GET /api/2.0/mlflow/experiments/search` (ดึง experiment list)
- `GET /api/admin/models` → `GET /api/2.0/mlflow/registered-models/search` (ดึง model list)

เพิ่ม `adminMiddleware()` ที่เช็ค JWT claim `is_admin == true` ก่อนอนุญาตเข้า `/api/admin/*`

---

### Svelte 5 Frontend

#### [MODIFY] [App.svelte](file:///home/d4c/Documents/Connectech/mlflow/portal/frontend/src/App.svelte)

เปลี่ยนจากหน้า Login อย่างเดียว เป็น SPA routing:
- `page === 'login'` → หน้า Login (เหมือนเดิม)
- `page === 'admin'` → หน้า Admin Panel (ใหม่)

เมื่อ Login สำเร็จเป็น admin จะมีปุ่ม "Admin Panel" ปรากฏ หรือ redirect ไปหน้า admin panel ก่อนเข้า MLflow

#### [NEW] [AdminPanel.svelte](file:///home/d4c/Documents/Connectech/mlflow/portal/frontend/src/AdminPanel.svelte)

หน้า Admin Panel หลัก ประกอบด้วย:
- **Header bar** — แสดงชื่อ user, ปุ่ม "Go to MLflow" และ "Logout"
- **Tab navigation** — 3 tabs: Users, Experiment Permissions, Model Permissions
- **Users tab** — ตารางแสดง users ทั้งหมด พร้อมปุ่ม: Create User, Toggle Admin, Reset Password, Delete
- **Experiment Permissions tab** — ตารางแสดง permissions ที่ assign ให้ user แต่ละคนต่อ experiment พร้อม dropdown เลือก Permission level (READ/EDIT/MANAGE/NO_PERMISSIONS)
- **Model Permissions tab** — เหมือน Experiment Permissions แต่สำหรับ Registered Models

Modal dialogs สำหรับ: สร้าง User ใหม่, Reset Password, Confirm Delete, Grant Permission

#### [MODIFY] [app.css](file:///home/d4c/Documents/Connectech/mlflow/portal/frontend/src/app.css)

เพิ่ม CSS สำหรับ Admin Panel: layout, tables, tabs, modals, badges ในธีม glassmorphism dark เดิม

---

## Verification Plan

### Manual Browser Testing
1. `cd /home/d4c/Documents/Connectech/mlflow && docker compose up --build -d`
2. เปิด browser ไปที่ `http://localhost`
3. Login ด้วย `admin` / `Connected@2022`
4. ตรวจสอบว่าเห็นปุ่ม "Admin Panel" (หรือถูก redirect ไปหน้า admin)
5. ทดสอบ CRUD Users: สร้าง user ใหม่, toggle admin, delete
6. ทดสอบ experiment permission: assign READ/EDIT/MANAGE ให้ user
7. ทดสอบ model permission: เหมือน experiment
8. Login เป็น non-admin user → ตรวจสอบว่าไม่สามารถเข้า admin panel ได้
