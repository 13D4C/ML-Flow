<script>
  // ─── State ───
  let page = $state('loading'); // 'loading' | 'login' | 'admin'
  let user = $state(null);

  // Login state
  let username = $state('');
  let password = $state('');
  let loginError = $state('');
  let loginLoading = $state(false);

  // Admin state
  let activeTab = $state('users');
  let users = $state([]);
  let experiments = $state([]);
  let models = $state([]);
  let loadingData = $state(false);
  let toast = $state(null);
  let toastTimeout = null;

  // Modal state
  let modal = $state(null); // { type: 'createUser' | 'resetPassword' | 'confirmDelete' | 'grantExpPerm' | 'grantModelPerm', data: {} }
  let modalForm = $state({});
  let modalLoading = $state(false);

  // ─── Init ───
  $effect(() => {
    checkAuth();
  });

  async function checkAuth() {
    try {
      const res = await fetch('/api/me');
      if (res.ok) {
        user = await res.json();
        if (user.is_admin) {
          page = 'admin';
          loadAllData();
        } else {
          window.location.href = '/mlflow/';
        }
      } else {
        page = 'login';
      }
    } catch {
      page = 'login';
    }
  }

  // ─── Toast ───
  function showToast(message, type = 'success') {
    if (toastTimeout) clearTimeout(toastTimeout);
    toast = { message, type };
    toastTimeout = setTimeout(() => { toast = null; }, 3500);
  }

  // ─── Login ───
  async function handleLogin(e) {
    e.preventDefault();
    loginError = '';
    loginLoading = true;
    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });
      const data = await res.json();
      if (res.ok) {
        user = data;
        if (data.is_admin) {
          page = 'admin';
          loadAllData();
        } else {
          window.location.href = '/mlflow/';
        }
      } else {
        loginError = data.error || 'Login failed.';
      }
    } catch {
      loginError = 'Cannot connect to server.';
    } finally {
      loginLoading = false;
    }
  }

  async function handleLogout() {
    await fetch('/api/logout', { method: 'POST' });
    user = null;
    page = 'login';
    username = '';
    password = '';
  }

  // ─── Data Loading ───
  async function loadAllData() {
    loadingData = true;
    await Promise.all([loadUsers(), loadExperiments(), loadModels()]);
    loadingData = false;
  }

  async function loadUsers() {
    try {
      const res = await fetch('/api/admin/users');
      if (res.ok) {
        const data = await res.json();
        users = data.users || [];
      }
    } catch {}
  }

  async function loadExperiments() {
    try {
      const res = await fetch('/api/admin/experiments');
      if (res.ok) {
        const data = await res.json();
        experiments = data.experiments || [];
      }
    } catch {}
  }

  async function loadModels() {
    try {
      const res = await fetch('/api/admin/models');
      if (res.ok) {
        const data = await res.json();
        models = data.registered_models || [];
      }
    } catch {}
  }

  async function searchUser(uname) {
    try {
      const res = await fetch('/api/admin/users/get?username=' + encodeURIComponent(uname));
      if (res.ok) {
        const data = await res.json();
        return data.user;
      }
    } catch {}
    return null;
  }

  async function refreshUser(uname) {
    const u = await searchUser(uname);
    if (u) {
      const idx = users.findIndex(x => x.username === uname);
      if (idx >= 0) {
        users[idx] = u;
        users = [...users];
      } else {
        users = [...users, u];
      }
    }
  }

  // ─── User Actions ───
  function openCreateUser() {
    modal = { type: 'createUser' };
    modalForm = { username: '', password: '' };
  }

  async function submitCreateUser() {
    if (!modalForm.username || !modalForm.password) return;
    modalLoading = true;
    try {
      const res = await fetch('/api/admin/users/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: modalForm.username, password: modalForm.password }),
      });
      if (res.ok) {
        showToast(`User "${modalForm.username}" created successfully`);
        await refreshUser(modalForm.username);
        modal = null;
      } else {
        const data = await res.json();
        showToast(data.error_code ? `${data.error_code}: ${data.message}` : (data.error || 'Failed to create user'), 'error');
      }
    } catch {
      showToast('Failed to create user', 'error');
    } finally {
      modalLoading = false;
    }
  }

  function openSearchUser() {
    modal = { type: 'searchUser' };
    modalForm = { username: '' };
  }

  async function submitSearchUser() {
    if (!modalForm.username) return;
    modalLoading = true;
    try {
      const u = await searchUser(modalForm.username);
      if (u) {
        const exists = users.find(x => x.username === u.username);
        if (!exists) {
          users = [...users, u];
        } else {
          // refresh the data
          users = users.map(x => x.username === u.username ? u : x);
        }
        showToast(`Found user "${u.username}"`);
        modal = null;
      } else {
        showToast('User not found', 'error');
      }
    } catch {
      showToast('Search failed', 'error');
    } finally {
      modalLoading = false;
    }
  }

  function openResetPassword(uname) {
    modal = { type: 'resetPassword', data: { username: uname } };
    modalForm = { password: '' };
  }

  async function submitResetPassword() {
    if (!modalForm.password) return;
    modalLoading = true;
    try {
      const res = await fetch('/api/admin/users/update-password', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: modal.data.username, password: modalForm.password }),
      });
      if (res.ok) {
        showToast(`Password updated for "${modal.data.username}"`);
        modal = null;
      } else {
        showToast('Failed to update password', 'error');
      }
    } catch {
      showToast('Failed to update password', 'error');
    } finally {
      modalLoading = false;
    }
  }

  function openDeleteUser(uname) {
    modal = { type: 'confirmDelete', data: { username: uname } };
  }

  async function submitDeleteUser() {
    modalLoading = true;
    try {
      const res = await fetch('/api/admin/users/delete', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: modal.data.username }),
      });
      if (res.ok) {
        showToast(`User "${modal.data.username}" deleted`);
        users = users.filter(u => u.username !== modal.data.username);
        modal = null;
      } else {
        showToast('Failed to delete user', 'error');
      }
    } catch {
      showToast('Failed to delete user', 'error');
    } finally {
      modalLoading = false;
    }
  }

  async function toggleAdmin(uname, currentAdmin) {
    try {
      const res = await fetch('/api/admin/users/update-admin', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: uname, is_admin: !currentAdmin }),
      });
      if (res.ok) {
        showToast(`${uname} is now ${!currentAdmin ? 'admin' : 'user'}`);
        await refreshUser(uname);
      } else {
        showToast('Failed to update admin status', 'error');
      }
    } catch {
      showToast('Failed to update admin status', 'error');
    }
  }

  // ─── Permission Actions ───
  function openGrantExpPerm() {
    modal = { type: 'grantExpPerm' };
    modalForm = { experiment_id: '', username: '', permission: 'READ' };
  }

  async function submitGrantExpPerm() {
    if (!modalForm.experiment_id || !modalForm.username || !modalForm.permission) return;
    modalLoading = true;
    try {
      const res = await fetch('/api/admin/experiment-permissions/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(modalForm),
      });
      if (res.ok) {
        showToast('Experiment permission granted');
        // Refresh user data to see permissions
        await refreshUser(modalForm.username);
        modal = null;
      } else {
        const data = await res.json();
        showToast(data.message || 'Failed to grant permission', 'error');
      }
    } catch {
      showToast('Failed to grant permission', 'error');
    } finally {
      modalLoading = false;
    }
  }

  async function updateExpPerm(experimentId, uname, permission) {
    try {
      const res = await fetch('/api/admin/experiment-permissions/update', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ experiment_id: experimentId, username: uname, permission }),
      });
      if (res.ok) {
        showToast('Permission updated');
        await refreshUser(uname);
      } else {
        showToast('Failed to update permission', 'error');
      }
    } catch {
      showToast('Failed to update permission', 'error');
    }
  }

  async function deleteExpPerm(experimentId, uname) {
    try {
      const res = await fetch('/api/admin/experiment-permissions/delete', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ experiment_id: experimentId, username: uname }),
      });
      if (res.ok) {
        showToast('Permission removed');
        await refreshUser(uname);
      } else {
        showToast('Failed to remove permission', 'error');
      }
    } catch {
      showToast('Failed to remove permission', 'error');
    }
  }

  function openGrantModelPerm() {
    modal = { type: 'grantModelPerm' };
    modalForm = { name: '', username: '', permission: 'READ' };
  }

  async function submitGrantModelPerm() {
    if (!modalForm.name || !modalForm.username || !modalForm.permission) return;
    modalLoading = true;
    try {
      const res = await fetch('/api/admin/model-permissions/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(modalForm),
      });
      if (res.ok) {
        showToast('Model permission granted');
        await refreshUser(modalForm.username);
        modal = null;
      } else {
        const data = await res.json();
        showToast(data.message || 'Failed to grant permission', 'error');
      }
    } catch {
      showToast('Failed to grant permission', 'error');
    } finally {
      modalLoading = false;
    }
  }

  async function updateModelPerm(modelName, uname, permission) {
    try {
      const res = await fetch('/api/admin/model-permissions/update', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: modelName, username: uname, permission }),
      });
      if (res.ok) {
        showToast('Permission updated');
        await refreshUser(uname);
      } else {
        showToast('Failed to update permission', 'error');
      }
    } catch {
      showToast('Failed to update permission', 'error');
    }
  }

  async function deleteModelPerm(modelName, uname) {
    try {
      const res = await fetch('/api/admin/model-permissions/delete', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: modelName, username: uname }),
      });
      if (res.ok) {
        showToast('Permission removed');
        await refreshUser(uname);
      } else {
        showToast('Failed to remove permission', 'error');
      }
    } catch {
      showToast('Failed to remove permission', 'error');
    }
  }

  // ─── Helpers ───
  const permissionLevels = ['READ', 'EDIT', 'MANAGE', 'NO_PERMISSIONS'];

  function getExpName(id) {
    const exp = experiments.find(e => e.experiment_id === id);
    return exp ? exp.name : `Experiment #${id}`;
  }

  // Collect all experiment permissions from all users
  function getAllExpPermissions() {
    const perms = [];
    for (const u of users) {
      if (u.experiment_permissions) {
        for (const p of u.experiment_permissions) {
          perms.push({ ...p, username: u.username });
        }
      }
    }
    return perms;
  }

  function getAllModelPermissions() {
    const perms = [];
    for (const u of users) {
      if (u.registered_model_permissions) {
        for (const p of u.registered_model_permissions) {
          perms.push({ ...p, username: u.username });
        }
      }
    }
    return perms;
  }
</script>

{#if page === 'loading'}
  <div class="page">
    <div class="loading-screen">
      <div class="spinner-lg"></div>
      <p>Loading...</p>
    </div>
  </div>

{:else if page === 'login'}
  <div class="page">
    <div class="particles">
      <div class="particle"></div><div class="particle"></div><div class="particle"></div>
      <div class="particle"></div><div class="particle"></div><div class="particle"></div>
      <div class="particle"></div><div class="particle"></div>
    </div>

    <div class="card">
      <div class="logo-area">
        <div class="logo-icon">M</div>
        <div class="logo-title">MLflow Portal</div>
        <div class="logo-subtitle">Machine Learning Lifecycle Platform</div>
      </div>

      <form onsubmit={handleLogin}>
        <div class="form-group">
          <label class="form-label" for="username">Username</label>
          <div class="input-wrapper">
            <span class="input-icon">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>
              </svg>
            </span>
            <input id="username" class="form-input" type="text" placeholder="Enter your username" bind:value={username} required autocomplete="username" disabled={loginLoading} />
          </div>
        </div>

        <div class="form-group">
          <label class="form-label" for="password">Password</label>
          <div class="input-wrapper">
            <span class="input-icon">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>
              </svg>
            </span>
            <input id="password" class="form-input" type="password" placeholder="Enter your password" bind:value={password} required autocomplete="current-password" disabled={loginLoading} />
          </div>
        </div>

        <button class="btn-login" type="submit" disabled={loginLoading}>
          {#if loginLoading}
            <span class="spinner"></span>Signing in...
          {:else}
            Sign In
          {/if}
        </button>

        {#if loginError}
          <div class="error-message">{loginError}</div>
        {/if}
      </form>
    </div>
  </div>

{:else if page === 'admin'}
  <div class="admin-layout">
    <!-- Header -->
    <header class="admin-header">
      <div class="header-left">
        <div class="header-logo">M</div>
        <div>
          <div class="header-title">MLflow Admin</div>
          <div class="header-sub">Role & Permission Management</div>
        </div>
      </div>
      <div class="header-right">
        <span class="header-user">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
          {user?.username}
        </span>
        <a href="/mlflow/" class="btn-header">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>
          MLflow UI
        </a>
        <button class="btn-header btn-logout" onclick={handleLogout}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/></svg>
          Logout
        </button>
      </div>
    </header>

    <!-- Tabs -->
    <nav class="admin-tabs">
      <button class="tab-btn" class:active={activeTab === 'users'} onclick={() => activeTab = 'users'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/></svg>
        Users
      </button>
      <button class="tab-btn" class:active={activeTab === 'experiments'} onclick={() => activeTab = 'experiments'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5H2v7l6.29 6.29c.94.94 2.48.94 3.42 0l4.58-4.58c.94-.94.94-2.48 0-3.42L9 5z"/><circle cx="6" cy="9" r="1"/></svg>
        Experiment Permissions
      </button>
      <button class="tab-btn" class:active={activeTab === 'models'} onclick={() => activeTab = 'models'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
        Model Permissions
      </button>
    </nav>

    <!-- Content -->
    <main class="admin-content">
      {#if loadingData}
        <div class="loading-screen">
          <div class="spinner-lg"></div>
          <p>Loading data...</p>
        </div>

      {:else if activeTab === 'users'}
        <!-- Users Tab -->
        <div class="tab-header">
          <h2>Users ({users.length})</h2>
          <div class="tab-actions">
            <button class="btn-action btn-secondary" onclick={openSearchUser}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg>
              Find User
            </button>
            <button class="btn-action btn-primary" onclick={openCreateUser}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><line x1="20" y1="8" x2="20" y2="14"/><line x1="23" y1="11" x2="17" y2="11"/></svg>
              Create User
            </button>
          </div>
        </div>

        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th>Username</th>
                <th>Role</th>
                <th>Experiment Perms</th>
                <th>Model Perms</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each users as u (u.username)}
                <tr>
                  <td class="td-user">
                    <div class="user-avatar">{u.username[0].toUpperCase()}</div>
                    <span>{u.username}</span>
                  </td>
                  <td>
                    <span class="badge" class:badge-admin={u.is_admin} class:badge-user={!u.is_admin}>
                      {u.is_admin ? 'Admin' : 'User'}
                    </span>
                  </td>
                  <td class="td-count">{u.experiment_permissions?.length || 0}</td>
                  <td class="td-count">{u.registered_model_permissions?.length || 0}</td>
                  <td class="td-actions">
                    <button class="btn-icon" title="Toggle Admin" onclick={() => toggleAdmin(u.username, u.is_admin)}>
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
                    </button>
                    <button class="btn-icon" title="Reset Password" onclick={() => openResetPassword(u.username)}>
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
                    </button>
                    {#if u.username !== user?.username}
                      <button class="btn-icon btn-icon-danger" title="Delete User" onclick={() => openDeleteUser(u.username)}>
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                      </button>
                    {/if}
                  </td>
                </tr>
              {/each}
              {#if users.length === 0}
                <tr><td colspan="5" class="td-empty">No users loaded. Use "Find User" to search.</td></tr>
              {/if}
            </tbody>
          </table>
        </div>

      {:else if activeTab === 'experiments'}
        <!-- Experiment Permissions Tab -->
        <div class="tab-header">
          <h2>Experiment Permissions</h2>
          <button class="btn-action btn-primary" onclick={openGrantExpPerm}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            Grant Permission
          </button>
        </div>

        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Experiment</th>
                <th>Permission</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each getAllExpPermissions() as p}
                <tr>
                  <td>{p.username}</td>
                  <td>{getExpName(p.experiment_id)}</td>
                  <td>
                    <select class="perm-select" value={p.permission} onchange={(e) => updateExpPerm(p.experiment_id, p.username, e.target.value)}>
                      {#each permissionLevels as level}
                        <option value={level} selected={p.permission === level}>{level}</option>
                      {/each}
                    </select>
                  </td>
                  <td>
                    <button class="btn-icon btn-icon-danger" title="Remove" onclick={() => deleteExpPerm(p.experiment_id, p.username)}>
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                    </button>
                  </td>
                </tr>
              {/each}
              {#if getAllExpPermissions().length === 0}
                <tr><td colspan="4" class="td-empty">No experiment permissions found. Add users first using "Find User".</td></tr>
              {/if}
            </tbody>
          </table>
        </div>

      {:else if activeTab === 'models'}
        <!-- Model Permissions Tab -->
        <div class="tab-header">
          <h2>Model Permissions</h2>
          <button class="btn-action btn-primary" onclick={openGrantModelPerm}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            Grant Permission
          </button>
        </div>

        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Model</th>
                <th>Permission</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each getAllModelPermissions() as p}
                <tr>
                  <td>{p.username}</td>
                  <td>{p.name}</td>
                  <td>
                    <select class="perm-select" value={p.permission} onchange={(e) => updateModelPerm(p.name, p.username, e.target.value)}>
                      {#each permissionLevels as level}
                        <option value={level} selected={p.permission === level}>{level}</option>
                      {/each}
                    </select>
                  </td>
                  <td>
                    <button class="btn-icon btn-icon-danger" title="Remove" onclick={() => deleteModelPerm(p.name, p.username)}>
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                    </button>
                  </td>
                </tr>
              {/each}
              {#if getAllModelPermissions().length === 0}
                <tr><td colspan="4" class="td-empty">No model permissions found. Add users first using "Find User".</td></tr>
              {/if}
            </tbody>
          </table>
        </div>
      {/if}
    </main>
  </div>
{/if}

<!-- Toast -->
{#if toast}
  <div class="toast" class:toast-error={toast.type === 'error'} class:toast-success={toast.type === 'success'}>
    {#if toast.type === 'success'}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>
    {:else}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
    {/if}
    {toast.message}
  </div>
{/if}

<!-- Modal Overlay -->
{#if modal}
  <div class="modal-overlay" onclick={() => { if (!modalLoading) modal = null; }}>
    <div class="modal-card" onclick={(e) => e.stopPropagation()}>

      {#if modal.type === 'createUser'}
        <h3 class="modal-title">Create New User</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitCreateUser(); }}>
          <div class="form-group">
            <label class="form-label">Username</label>
            <input class="form-input modal-input" type="text" bind:value={modalForm.username} placeholder="Enter username" required disabled={modalLoading} />
          </div>
          <div class="form-group">
            <label class="form-label">Password</label>
            <input class="form-input modal-input" type="password" bind:value={modalForm.password} placeholder="Enter password" required disabled={modalLoading} />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-action btn-secondary" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="btn-action btn-primary" disabled={modalLoading}>
              {#if modalLoading}<span class="spinner"></span>{/if}Create
            </button>
          </div>
        </form>

      {:else if modal.type === 'searchUser'}
        <h3 class="modal-title">Find User</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitSearchUser(); }}>
          <div class="form-group">
            <label class="form-label">Username</label>
            <input class="form-input modal-input" type="text" bind:value={modalForm.username} placeholder="Enter exact username" required disabled={modalLoading} />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-action btn-secondary" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="btn-action btn-primary" disabled={modalLoading}>
              {#if modalLoading}<span class="spinner"></span>{/if}Search
            </button>
          </div>
        </form>

      {:else if modal.type === 'resetPassword'}
        <h3 class="modal-title">Reset Password — {modal.data.username}</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitResetPassword(); }}>
          <div class="form-group">
            <label class="form-label">New Password</label>
            <input class="form-input modal-input" type="password" bind:value={modalForm.password} placeholder="Enter new password" required disabled={modalLoading} />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-action btn-secondary" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="btn-action btn-primary" disabled={modalLoading}>
              {#if modalLoading}<span class="spinner"></span>{/if}Update
            </button>
          </div>
        </form>

      {:else if modal.type === 'confirmDelete'}
        <h3 class="modal-title">Delete User</h3>
        <p class="modal-text">Are you sure you want to delete <strong>{modal.data.username}</strong>? This action cannot be undone.</p>
        <div class="modal-actions">
          <button class="btn-action btn-secondary" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
          <button class="btn-action btn-danger" onclick={submitDeleteUser} disabled={modalLoading}>
            {#if modalLoading}<span class="spinner"></span>{/if}Delete
          </button>
        </div>

      {:else if modal.type === 'grantExpPerm'}
        <h3 class="modal-title">Grant Experiment Permission</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitGrantExpPerm(); }}>
          <div class="form-group">
            <label class="form-label">Username</label>
            <input class="form-input modal-input" type="text" bind:value={modalForm.username} placeholder="Enter username" required disabled={modalLoading} />
          </div>
          <div class="form-group">
            <label class="form-label">Experiment</label>
            {#if experiments.length > 0}
              <select class="form-input modal-input" bind:value={modalForm.experiment_id} required disabled={modalLoading}>
                <option value="">Select experiment...</option>
                {#each experiments as exp}
                  <option value={exp.experiment_id}>{exp.name} (#{exp.experiment_id})</option>
                {/each}
              </select>
            {:else}
              <input class="form-input modal-input" type="text" bind:value={modalForm.experiment_id} placeholder="Experiment ID" required disabled={modalLoading} />
            {/if}
          </div>
          <div class="form-group">
            <label class="form-label">Permission</label>
            <select class="form-input modal-input" bind:value={modalForm.permission} required disabled={modalLoading}>
              {#each permissionLevels as level}
                <option value={level}>{level}</option>
              {/each}
            </select>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-action btn-secondary" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="btn-action btn-primary" disabled={modalLoading}>
              {#if modalLoading}<span class="spinner"></span>{/if}Grant
            </button>
          </div>
        </form>

      {:else if modal.type === 'grantModelPerm'}
        <h3 class="modal-title">Grant Model Permission</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitGrantModelPerm(); }}>
          <div class="form-group">
            <label class="form-label">Username</label>
            <input class="form-input modal-input" type="text" bind:value={modalForm.username} placeholder="Enter username" required disabled={modalLoading} />
          </div>
          <div class="form-group">
            <label class="form-label">Model</label>
            {#if models.length > 0}
              <select class="form-input modal-input" bind:value={modalForm.name} required disabled={modalLoading}>
                <option value="">Select model...</option>
                {#each models as m}
                  <option value={m.name}>{m.name}</option>
                {/each}
              </select>
            {:else}
              <input class="form-input modal-input" type="text" bind:value={modalForm.name} placeholder="Model name" required disabled={modalLoading} />
            {/if}
          </div>
          <div class="form-group">
            <label class="form-label">Permission</label>
            <select class="form-input modal-input" bind:value={modalForm.permission} required disabled={modalLoading}>
              {#each permissionLevels as level}
                <option value={level}>{level}</option>
              {/each}
            </select>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-action btn-secondary" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="btn-action btn-primary" disabled={modalLoading}>
              {#if modalLoading}<span class="spinner"></span>{/if}Grant
            </button>
          </div>
        </form>
      {/if}
    </div>
  </div>
{/if}
