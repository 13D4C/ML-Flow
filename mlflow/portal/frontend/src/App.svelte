<script>
  // ─── State ───
  let page = $state('loading'); // 'loading' | 'landing' | 'login' | 'admin' | 'oidcCallback'
  let user = $state(null);

  // Login state
  let username = $state('');
  let password = $state('');
  let loginError = $state('');
  let loginLoading = $state(false);
  let showPassword = $state(false);
  let oidcEnabled = $state(false);
  let oidcCallbackError = $state('');

  // Admin state
  let activeTab = $state('users');
  let users = $state([]);
  let experiments = $state([]);
  let models = $state([]);
  let loadingData = $state(false);
  let toast = $state(null);
  let toastTimeout = null;

  // Modal state
  let modal = $state(null);
  let modalForm = $state({});
  let modalLoading = $state(false);

  // ─── Init ───
  $effect(() => {
    // Handle OIDC callback route (e.g. /entry/callback?code=...&state=...)
    if (window.location.pathname === '/entry/callback') {
      page = 'oidcCallback';
      handleOIDCCallback();
      return;
    }
    checkAuth();
  });

  async function handleOIDCCallback() {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    const state = params.get('state');
    const error = params.get('error');

    if (error) {
      oidcCallbackError = params.get('error_description') || error;
      return;
    }
    if (!code || !state) {
      oidcCallbackError = 'Missing code or state in callback URL';
      return;
    }

    try {
      const res = await fetch(`/api/oidc/exchange?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`);
      const data = await res.json();
      if (res.ok && data.ok) {
        window.location.href = data.is_admin ? '/' : '/mlflow/';
      } else {
        oidcCallbackError = data.error || 'Authentication failed';
      }
    } catch {
      oidcCallbackError = 'Cannot connect to server';
    }
  }

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
        page = 'landing';
      }
    } catch {
      page = 'landing';
    }

    try {
      const oidcRes = await fetch('/api/oidc/enabled');
      if (oidcRes.ok) {
        const data = await oidcRes.json();
        oidcEnabled = data.enabled === true;
      }
    } catch (e) {
      console.log('OIDC check failed:', e);
    }
  }

  function goToLogin() {
    if (oidcEnabled) {
      window.location.href = '/api/oidc/login';
    } else {
      page = 'login';
    }
  }
  function goToLanding() { page = 'landing'; }

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
    page = 'landing';
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

  const tabTitles = {
    users: 'User Management',
    experiments: 'Experiment Permissions',
    models: 'Model Permissions',
  };
</script>

<!-- ============================================= -->
<!-- LOADING                                       -->
<!-- ============================================= -->
{#if page === 'loading'}
  <div class="flex items-center justify-center min-h-screen bg-brand-bg">
    <div class="w-8 h-8 rounded-full border-2 border-accent/20 border-t-accent animate-spin"></div>
  </div>

<!-- ============================================= -->
<!-- OIDC CALLBACK                                -->
<!-- ============================================= -->
{:else if page === 'oidcCallback'}
  <div class="flex items-center justify-center min-h-screen bg-brand-bg px-4">
    <div class="text-center">
      {#if oidcCallbackError}
        <div class="w-12 h-12 rounded-full bg-red-500/10 border border-red-500/20 flex items-center justify-center mx-auto mb-4">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
        </div>
        <p class="text-white font-semibold mb-1">Authentication Failed</p>
        <p class="text-brand-muted text-sm mb-5">{oidcCallbackError}</p>
        <a href="/" class="px-5 py-2 rounded-md text-sm font-semibold bg-accent text-brand-bg hover:brightness-110 transition-all">Back to Home</a>
      {:else}
        <div class="w-10 h-10 rounded-full border-2 border-accent/20 border-t-accent animate-spin mx-auto mb-4"></div>
        <p class="text-white font-semibold mb-1">Signing you in…</p>
        <p class="text-brand-muted text-sm">Please wait while we complete authentication</p>
      {/if}
    </div>
  </div>

<!-- ============================================= -->
<!-- LANDING PAGE                                  -->
<!-- ============================================= -->
{:else if page === 'landing'}
  <div class="min-h-screen bg-brand-bg overflow-x-hidden pt-14">
    <nav class="fixed top-0 inset-x-0 z-50 flex items-center justify-between px-10 h-14 bg-brand-bg/85 backdrop-blur-md border-b border-white/5">
      <a class="flex items-center no-underline" href="/">
        <img src="/logo.png" alt="ConnectedTech" class="h-7 brightness-110" />
      </a>
      <div class="flex items-center gap-2">
        <a class="px-4 py-1.5 rounded-md text-[13px] font-medium border border-white/5 text-brand-muted hover:border-accent/20 hover:text-slate-100 transition-colors flex items-center gap-1.5" href="https://mlflow.org/docs/latest/index.html" target="_blank" rel="noopener">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/></svg>
          Docs
        </a>
        <button class="px-4 py-1.5 rounded-md text-[13px] font-semibold bg-accent text-brand-bg hover:brightness-110 transition-all flex items-center gap-1.5" onclick={goToLogin}>
          Sign In
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
        </button>
      </div>
    </nav>

    <section class="relative pt-[72px] pb-12 px-10 max-w-6xl mx-auto grid md:grid-cols-2 gap-12 items-center">
      <!-- Glow before backdrop -->
      <div class="absolute -top-[80px] -left-[160px] w-[480px] h-[480px] bg-accent/5 rounded-full blur-[100px] pointer-events-none"></div>
      
      <div class="relative z-10">
        <div class="inline-flex items-center gap-1.5 px-3 py-1.5 bg-accent/10 border border-accent/20 rounded-full text-xs font-medium text-accent mb-5">
          <span class="w-[5px] h-[5px] bg-accent rounded-full animate-blink"></span>
          MLflow Platform — Powered by ConnectedTech
        </div>
        <h1 class="text-[38px] md:text-[46px] font-extrabold tracking-[-1.5px] leading-[1.08] mb-4 text-white">
          Track, manage, and <span class="bg-gradient-to-br from-accent to-[#00B8A8] bg-clip-text text-transparent">deploy ML models</span> at scale.
        </h1>
        <p class="text-base text-brand-muted max-w-md leading-relaxed mb-7">
          A secure, self-hosted MLflow platform with built-in authentication, experiment tracking, model registry, 
          and artifact storage — all managed through an elegant admin portal.
        </p>
        <div class="flex flex-wrap gap-2.5">
          <button class="px-6 py-2.5 rounded-md text-sm font-semibold bg-accent text-brand-bg hover:brightness-110 hover:-translate-y-[1px] transition-all flex items-center gap-1.5 shadow-[0_4px_20px_rgba(0,229,208,0.25)]" onclick={goToLogin}>
            Get Started
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
          </button>
          <a class="px-6 py-2.5 rounded-md text-sm font-semibold border border-white/10 text-slate-100 hover:border-accent/20 hover:bg-white/5 transition-colors flex items-center gap-1.5" href="https://mlflow.org/docs/latest/index.html" target="_blank" rel="noopener">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/></svg>
            MLflow Documentation
          </a>
        </div>
      </div>
      
      <div class="relative z-10 w-full">
        <div class="absolute w-[260px] h-[260px] -top-10 -right-8 bg-[radial-gradient(circle,rgba(0,229,208,0.05)_0%,transparent_70%)] rounded-full animate-orb1 pointer-events-none"></div>
        <div class="absolute w-[180px] h-[180px] -bottom-5 right-20 bg-[radial-gradient(circle,rgba(0,229,208,0.03)_0%,transparent_70%)] rounded-full animate-orb2 pointer-events-none"></div>
        
        <div class="bg-brand-surface border border-white/5 rounded-lg overflow-hidden shadow-[0_16px_48px_rgba(0,0,0,0.4)] md:animate-float">
          <div class="flex items-center gap-1.5 px-3.5 py-2.5 bg-white/5 border-b border-white/5">
            <div class="w-2.5 h-2.5 rounded-full bg-red-500"></div>
            <div class="w-2.5 h-2.5 rounded-full bg-amber-500"></div>
            <div class="w-2.5 h-2.5 rounded-full bg-green-500"></div>
            <span class="ml-2.5 text-[11px] text-brand-subtle px-2.5 py-1 bg-white/5 rounded">train.py</span>
          </div>
          <div class="p-4 font-mono text-[12px] leading-[1.9] overflow-x-auto whitespace-pre">
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">1</span><span><span class="text-purple-400">import</span> <span class="text-accent">mlflow</span></span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">2</span><span></span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">3</span><span><span class="text-brand-subtle italic"># Set tracking server</span></span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">4</span><span><span class="text-accent">mlflow</span>.<span class="text-blue-400">set_tracking_uri</span>(<span class="text-emerald-400">"https://ml.connectedtech.co"</span>)</span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">5</span><span><span class="text-accent">mlflow</span>.<span class="text-blue-400">set_experiment</span>(<span class="text-emerald-400">"tape-inspection-v2"</span>)</span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">6</span><span></span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">7</span><span><span class="text-purple-400">with</span> <span class="text-accent">mlflow</span>.<span class="text-blue-400">start_run</span>():</span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">8</span><span>    <span class="text-accent">mlflow</span>.<span class="text-blue-400">log_param</span>(<span class="text-emerald-400">"model"</span>, <span class="text-emerald-400">"yolov8"</span>)</span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">9</span><span>    <span class="text-accent">mlflow</span>.<span class="text-blue-400">log_metric</span>(<span class="text-emerald-400">"accuracy"</span>, <span class="text-orange-400">0.97</span>)</span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">10</span><span>    <span class="text-accent">mlflow</span>.<span class="text-blue-400">log_artifact</span>(<span class="text-emerald-400">"./model.pt"</span>)</span></div>
<div class="flex gap-3"><span class="text-brand-subtle select-none min-w-[20px] text-right">11</span><span>    <span class="text-purple-400">print</span>(<span class="text-emerald-400">"✓ Run logged successfully"</span>)</span></div>
          </div>
        </div>
      </div>
    </section>

    <section class="py-12 px-10 max-w-6xl mx-auto">
      <p class="text-[11px] font-semibold text-accent uppercase tracking-[1.5px] mb-2">Platform Features</p>
      <h2 class="text-[28px] font-bold tracking-[-0.5px] mb-3 text-white">Everything you need for ML lifecycle</h2>
      <p class="text-[15px] text-brand-muted max-w-[480px] leading-relaxed mb-9">
        A complete, self-hosted MLflow setup with enterprise-grade authentication and fine-grained permission control.
      </p>
      <div class="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Cards -->
        {#each [
          { icon: 'M22 12h-4l-3 9L9 3l-3 9H2', title: 'Experiment Tracking', desc: 'Log parameters, metrics, and artifacts for every training run. Compare results.' },
          { icon: 'M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z M3.27 6.96 12 12.01 20.73 6.96 M12 22.08 12 12', title: 'Model Registry', desc: 'Version, stage, and manage your ML models. Seamless transition from dev to prod.' },
          { icon: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M17 8 12 3 7 8 M12 3 12 15', title: 'Artifact Storage', desc: 'Store models, datasets, and files in S3-compatible storage with MinIO.' },
          { icon: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z', title: 'Auth & Permissions', desc: 'Fine-grained role-based access control. Manage view/edit/manage rights.' }
        ] as feature}
        <div class="p-5 bg-white/5 border border-white/5 rounded-lg transition-all hover:border-accent/20 hover:-translate-y-0.5 hover:shadow-[0_0_24px_rgba(0,229,208,0.08)]">
          <div class="w-10 h-10 bg-accent/10 rounded-md flex items-center justify-center mb-3.5 text-accent">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              {#each feature.icon.split(' M') as path, i}
                <path d={i === 0 ? path : 'M' + path}/>
              {/each}
            </svg>
          </div>
          <div class="text-[14px] font-semibold mb-1.5 text-white">{feature.title}</div>
          <p class="text-[13px] text-brand-muted leading-[1.55]">{feature.desc}</p>
        </div>
        {/each}
      </div>
    </section>

    <footer class="py-6 px-10 border-t border-white/5 flex flex-col md:flex-row items-center justify-between max-w-6xl mx-auto text-brand-subtle">
      <p class="text-xs">© 2026 ConnectedTech Co., Ltd. All rights reserved.</p>
      <div class="flex gap-4 mt-2 md:mt-0">
        <a class="text-xs text-brand-muted hover:text-accent transition-colors" href="https://mlflow.org" target="_blank" rel="noopener">MLflow</a>
        <a class="text-xs text-brand-muted hover:text-accent transition-colors" href="https://mlflow.org/docs/latest/index.html" target="_blank" rel="noopener">Documentation</a>
      </div>
    </footer>
  </div>

<!-- ============================================= -->
<!-- LOGIN PAGE — Minimalist Centered Card         -->
<!-- ============================================= -->
{:else if page === 'login'}
  <div class="min-h-screen flex items-center justify-center relative bg-brand-bg px-4 py-8">
    <div class="absolute inset-0 pointer-events-none bg-[radial-gradient(ellipse_at_50%_0%,rgba(0,229,208,0.04)_0%,transparent_60%)]"></div>

    <div class="relative w-full max-w-[380px] px-9 pt-10 pb-8 bg-brand-surface border border-white/10 rounded-xl shadow-[0_20px_60px_rgba(0,0,0,0.5)] z-10 animate-[cardUp_0.4s_ease-out_both] overflow-hidden">
      <!-- Logo -->
      <div class="flex justify-center mb-7">
        <img src="/logo.png" alt="ConnectedTech" class="h-9 brightness-110" />
      </div>

      <!-- Title -->
      <div class="text-center mb-7">
        <h1 class="text-[22px] font-bold tracking-[-0.3px] mb-1 text-white">Sign in</h1>
        <p class="text-[13px] text-brand-subtle">to continue to MLflow Platform</p>
      </div>

      <!-- Form -->
      <form onsubmit={handleLogin} autocomplete="on">
        <div class="mb-4">
          <label class="block text-[12px] font-medium text-brand-muted mb-1.5 tracking-[0.2px]" for="lg-user">Username</label>
          <input id="lg-user" class="w-full px-3 py-2.5 bg-brand-elevated border border-white/5 rounded-md text-[14px] text-white outline-none transition-all hover:border-white/10 focus:border-accent focus:shadow-[0_0_0_2px_rgba(0,229,208,0.08)]" type="text" bind:value={username} required autocomplete="username" disabled={loginLoading} placeholder=" " />
        </div>

        <div class="mb-4 relative">
          <label class="block text-[12px] font-medium text-brand-muted mb-1.5 tracking-[0.2px]" for="lg-pass">Password</label>
          <input id="lg-pass" class="w-full pl-3 pr-10 py-2.5 bg-brand-elevated border border-white/5 rounded-md text-[14px] text-white outline-none transition-all hover:border-white/10 focus:border-accent focus:shadow-[0_0_0_2px_rgba(0,229,208,0.08)]" type={showPassword ? 'text' : 'password'} bind:value={password} required autocomplete="current-password" disabled={loginLoading} placeholder=" " />
          <button type="button" class="absolute right-2.5 top-[30px] p-0.5 mt-0.5 text-brand-subtle hover:text-brand-muted transition-colors" onclick={() => showPassword = !showPassword} tabindex="-1">
            {#if showPassword}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>
            {:else}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
            {/if}
          </button>
        </div>

        {#if loginError}
          <div class="px-3 py-2 bg-red-dim border border-red-500/15 rounded-md text-red-300 text-[12px] text-center mb-1 animate-[shake_0.3s_ease]">{loginError}</div>
        {/if}

        <button class="w-full py-2.5 mt-2 bg-accent text-brand-bg rounded-md text-[14px] font-semibold flex items-center justify-center hover:brightness-110 transition-all disabled:opacity-50 disabled:cursor-not-allowed h-10" type="submit" disabled={loginLoading}>
          {#if loginLoading}
            <span class="w-4 h-4 rounded-full border-2 border-brand-bg/30 border-t-brand-bg animate-spin"></span>
          {:else}
            Continue
          {/if}
        </button>
      </form>

      <!-- Divider -->
      {#if oidcEnabled}
      <div class="flex items-center gap-3 my-6">
        <div class="h-px bg-white/5 flex-1"></div>
        <span class="text-[11px] text-brand-subtle uppercase tracking-wider font-medium">Or</span>
        <div class="h-px bg-white/5 flex-1"></div>
      </div>

      <!-- OIDC Login Button -->
      <a href="/api/oidc/login" class="w-full py-2.5 bg-white/5 hover:bg-white/10 text-white rounded-md text-[14px] font-medium flex items-center justify-center gap-2 transition-all border border-white/10 hover:border-white/20 h-10 mb-6">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="opacity-80"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/><polyline points="10 17 15 12 10 7"/><line x1="15" y1="12" x2="3" y2="12"/></svg>
        Sign in with ConnectedTech
      </a>
      {:else}
      <div class="h-px bg-white/5 my-6"></div>
      {/if}

      <button class="w-full flex items-center justify-center gap-1.5 py-2 px-2 border border-white/5 rounded-md text-brand-subtle text-[13px] hover:text-brand-muted hover:border-white/10 transition-all" onclick={goToLanding}>
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>
        Back to home
      </button>
    </div>
  </div>

<!-- ============================================= -->
<!-- ADMIN PANEL                                   -->
<!-- ============================================= -->
{:else if page === 'admin'}
  <div class="flex min-h-screen bg-brand-bg">
    <!-- Sidebar -->
    <aside class="fixed inset-y-0 left-0 z-50 w-[240px] hidden md:flex flex-col bg-brand-surface border-r border-brand-border">
      <div class="flex items-center gap-2.5 px-4 pt-4 pb-3.5 border-b border-brand-border">
        <img src="/logo.png" alt="ConnectedTech" class="h-6 brightness-110" />
        <span class="px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.5px] text-accent bg-accent-dim rounded-[3px]">Admin</span>
      </div>

      <nav class="flex-1 px-2 py-3 overflow-y-auto">
        <div class="px-2.5 pt-2.5 pb-1 text-[10px] font-semibold text-brand-subtle uppercase tracking-widest">Management</div>
        <button class="w-full flex items-center gap-2 px-2.5 py-2 rounded text-[13px] font-medium text-brand-muted hover:text-slate-100 hover:bg-white/5 transition-colors text-left {activeTab === 'users' ? '!text-accent !bg-accent-dim' : ''}" onclick={() => activeTab = 'users'}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/></svg>
          Users
        </button>
        <div class="px-2.5 pt-2.5 pb-1 text-[10px] font-semibold text-brand-subtle uppercase tracking-widest mt-2">Permissions</div>
        <button class="w-full flex items-center gap-2 px-2.5 py-2 rounded text-[13px] font-medium text-brand-muted hover:text-slate-100 hover:bg-white/5 transition-colors text-left {activeTab === 'experiments' ? '!text-accent !bg-accent-dim' : ''}" onclick={() => activeTab = 'experiments'}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>
          Experiments
        </button>
        <button class="w-full flex items-center gap-2 px-2.5 py-2 rounded text-[13px] font-medium text-brand-muted hover:text-slate-100 hover:bg-white/5 transition-colors text-left {activeTab === 'models' ? '!text-accent !bg-accent-dim' : ''}" onclick={() => activeTab = 'models'}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
          Models
        </button>
      </nav>

      <div class="p-2 border-t border-brand-border">
        <div class="flex items-center gap-2 px-2.5 py-2 mb-1.5 bg-white/5 border border-white/5 rounded-md">
          <div class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded bg-accent text-brand-bg text-[12px] font-bold">
            {user?.username?.[0]?.toUpperCase() || 'A'}
          </div>
          <div class="flex flex-col min-w-0">
            <span class="text-[13px] font-medium text-slate-100 truncate">{user?.username}</span>
            <span class="text-[10px] text-brand-subtle">Administrator</span>
          </div>
        </div>
        <a href="/mlflow/" class="w-full flex items-center gap-1.5 px-2.5 py-1.5 rounded text-[12px] font-medium text-brand-subtle hover:text-brand-muted hover:bg-white/5 transition-colors">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>
          Open MLflow
        </a>
        <button class="w-full flex items-center gap-1.5 px-2.5 py-1.5 rounded text-[12px] font-medium text-brand-subtle hover:text-red-300 hover:bg-red-dim transition-colors text-left" onclick={handleLogout}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/></svg>
          Sign Out
        </button>
      </div>
    </aside>

    <!-- Main Content -->
    <div class="flex-1 flex flex-col md:ml-[240px] min-h-screen">
      <!-- Topbar -->
      <header class="sticky top-0 z-40 flex items-center justify-between px-4 py-2.5 md:px-7 md:py-3.5 bg-brand-bg/90 backdrop-blur-md border-b border-brand-border">
        <h1 class="text-base font-bold tracking-tight text-white">{tabTitles[activeTab]}</h1>
        <div class="flex gap-2">
          {#if activeTab === 'users'}
            <button class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-md text-[12px] font-semibold border border-white/5 bg-white/5 text-brand-muted hover:border-white/10 hover:text-white transition-colors" onclick={openSearchUser}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg>
              Find User
            </button>
            <button class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-md text-[12px] font-semibold bg-accent text-brand-bg hover:brightness-110 transition-colors" onclick={openCreateUser}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              Create User
            </button>
          {:else}
            <button class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-md text-[12px] font-semibold bg-accent text-brand-bg hover:brightness-110 transition-colors" onclick={activeTab === 'experiments' ? openGrantExpPerm : openGrantModelPerm}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              Grant Permission
            </button>
          {/if}
        </div>
      </header>

      <!-- Body -->
      <main class="flex-1 p-4 md:p-7 animate-[fadeUp_0.25s_ease]">
        {#if loadingData}
          <div class="flex items-center justify-center min-h-[400px]">
            <div class="w-8 h-8 rounded-full border-2 border-accent/20 border-t-accent animate-spin"></div>
          </div>

        {:else if activeTab === 'users'}
          <!-- KPI Row -->
          <div class="grid grid-cols-1 md:grid-cols-3 gap-3 mb-5">
            <div class="px-5 py-4 bg-white/5 border border-white/5 rounded-[6px] hover:border-accent/20 transition-colors">
              <div class="text-[28px] font-extrabold tracking-tight leading-none text-white">{users.length}</div>
              <div class="text-[11px] font-medium text-brand-subtle uppercase tracking-wide mt-1">Users</div>
            </div>
            <div class="px-5 py-4 bg-white/5 border border-white/5 rounded-[6px] hover:border-accent/20 transition-colors">
              <div class="text-[28px] font-extrabold tracking-tight leading-none text-white">{experiments.length}</div>
              <div class="text-[11px] font-medium text-brand-subtle uppercase tracking-wide mt-1">Experiments</div>
            </div>
            <div class="px-5 py-4 bg-white/5 border border-white/5 rounded-[6px] hover:border-accent/20 transition-colors">
              <div class="text-[28px] font-extrabold tracking-tight leading-none text-white">{models.length}</div>
              <div class="text-[11px] font-medium text-brand-subtle uppercase tracking-wide mt-1">Models</div>
            </div>
          </div>

          <!-- Table -->
          <div class="bg-white/5 border border-white/5 rounded-md overflow-x-auto shadow-sm">
            <table class="w-full text-left border-collapse whitespace-nowrap">
              <thead class="bg-white/5">
                <tr>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">User</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Role</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Exp Perms</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Model Perms</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-white/5">
                {#each users as u (u.username)}
                  <tr class="hover:bg-white/5 transition-colors">
                    <td class="px-4 py-2.5 text-[13px] text-white">
                      <div class="flex items-center gap-2.5">
                        <div class="w-7 h-7 rounded bg-accent text-brand-bg flex items-center justify-center text-[11px] font-bold flex-shrink-0">{u.username[0].toUpperCase()}</div>
                        {u.username}
                      </div>
                    </td>
                    <td class="px-4 py-2.5">
                      <span class="inline-flex px-2.5 py-0.5 rounded-[3px] text-[11px] font-semibold tracking-wide border {u.is_admin ? 'bg-accent-dim text-accent border-accent/20' : 'bg-white/5 text-brand-muted border-transparent'}">
                        {u.is_admin ? 'Admin' : 'User'}
                      </span>
                    </td>
                    <td class="px-4 py-2.5 text-[13px] text-brand-muted tabular-nums">{u.experiment_permissions?.length || 0}</td>
                    <td class="px-4 py-2.5 text-[13px] text-brand-muted tabular-nums">{u.registered_model_permissions?.length || 0}</td>
                    <td class="px-4 py-2.5">
                      <div class="flex justify-end gap-1">
                        <button class="w-7 h-7 flex items-center justify-center rounded bg-transparent border border-white/5 text-brand-subtle hover:text-accent hover:border-accent/20 hover:bg-accent-glow transition-colors" title="Toggle Admin" onclick={() => toggleAdmin(u.username, u.is_admin)}>
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
                        </button>
                        <button class="w-7 h-7 flex items-center justify-center rounded bg-transparent border border-white/5 text-brand-subtle hover:text-accent hover:border-accent/20 hover:bg-accent-glow transition-colors" title="Reset Password" onclick={() => openResetPassword(u.username)}>
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
                        </button>
                        {#if u.username !== user?.username}
                          <button class="w-7 h-7 flex items-center justify-center rounded bg-transparent border border-white/5 text-brand-subtle hover:text-red-500 hover:border-red-500/20 hover:bg-red-dim transition-colors" title="Delete" onclick={() => openDeleteUser(u.username)}>
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                          </button>
                        {/if}
                      </div>
                    </td>
                  </tr>
                {/each}
                {#if users.length === 0}
                  <tr><td colspan="5" class="px-4 py-10 text-center text-[13px] text-brand-subtle">No users loaded</td></tr>
                {/if}
              </tbody>
            </table>
          </div>

        {:else if activeTab === 'experiments'}
          <div class="bg-white/5 border border-white/5 rounded-md overflow-x-auto shadow-sm">
            <table class="w-full text-left border-collapse whitespace-nowrap">
              <thead class="bg-white/5">
                <tr>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">User</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Experiment</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Permission</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-white/5">
                {#each getAllExpPermissions() as p}
                  <tr class="hover:bg-white/5 transition-colors">
                    <td class="px-4 py-2.5 text-[13px] text-white">{p.username}</td>
                    <td class="px-4 py-2.5 text-[13px] text-white">{getExpName(p.experiment_id)}</td>
                    <td class="px-4 py-2.5">
                      <select class="w-36 px-2.5 py-1.5 bg-brand-elevated border border-white/5 rounded text-[12px] text-white outline-none appearance-none cursor-pointer hover:border-white/10 focus:border-accent transition-colors bg-[url('data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20width%3D%2210%22%20height%3D%2210%22%20viewBox%3D%220%200%2024%2024%22%20fill%3D%22none%22%20stroke%3D%22%2394A3B8%22%20stroke-width%3D%222%22%3E%3Cpolyline%20points%3D%226%209%2012%2015%2018%209%22%2F%3E%3C%2Fsvg%3E')] bg-[position:calc(100%-8px)_center] bg-no-repeat" value={p.permission} onchange={(e) => updateExpPerm(p.experiment_id, p.username, e.target.value)}>
                        {#each permissionLevels as level}<option value={level} class="bg-brand-elevated text-white">{level}</option>{/each}
                      </select>
                    </td>
                    <td class="px-4 py-2.5 text-right">
                      <button class="w-7 h-7 inline-flex items-center justify-center rounded bg-transparent border border-white/5 text-brand-subtle hover:text-red-500 hover:border-red-500/20 hover:bg-red-dim transition-colors" title="Remove" onclick={() => deleteExpPerm(p.experiment_id, p.username)}>
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                      </button>
                    </td>
                  </tr>
                {/each}
                {#if getAllExpPermissions().length === 0}
                  <tr><td colspan="4" class="px-4 py-10 text-center text-[13px] text-brand-subtle">No permissions found</td></tr>
                {/if}
              </tbody>
            </table>
          </div>

        {:else if activeTab === 'models'}
          <div class="bg-white/5 border border-white/5 rounded-md overflow-x-auto shadow-sm">
            <table class="w-full text-left border-collapse whitespace-nowrap">
              <thead class="bg-white/5">
                <tr>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">User</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Model</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5">Permission</th>
                  <th class="px-4 py-2.5 text-[10px] font-semibold text-brand-subtle uppercase tracking-[0.8px] border-b border-white/5"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-white/5">
                {#each getAllModelPermissions() as p}
                  <tr class="hover:bg-white/5 transition-colors">
                    <td class="px-4 py-2.5 text-[13px] text-white">{p.username}</td>
                    <td class="px-4 py-2.5 text-[13px] text-white">{p.name}</td>
                    <td class="px-4 py-2.5">
                      <select class="w-36 px-2.5 py-1.5 bg-brand-elevated border border-white/5 rounded text-[12px] text-white outline-none appearance-none cursor-pointer hover:border-white/10 focus:border-accent transition-colors bg-[url('data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20width%3D%2210%22%20height%3D%2210%22%20viewBox%3D%220%200%2024%2024%22%20fill%3D%22none%22%20stroke%3D%22%2394A3B8%22%20stroke-width%3D%222%22%3E%3Cpolyline%20points%3D%226%209%2012%2015%2018%209%22%2F%3E%3C%2Fsvg%3E')] bg-[position:calc(100%-8px)_center] bg-no-repeat" value={p.permission} onchange={(e) => updateModelPerm(p.name, p.username, e.target.value)}>
                        {#each permissionLevels as level}<option value={level} class="bg-brand-elevated text-white">{level}</option>{/each}
                      </select>
                    </td>
                    <td class="px-4 py-2.5 text-right">
                      <button class="w-7 h-7 inline-flex items-center justify-center rounded bg-transparent border border-white/5 text-brand-subtle hover:text-red-500 hover:border-red-500/20 hover:bg-red-dim transition-colors" title="Remove" onclick={() => deleteModelPerm(p.name, p.username)}>
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                      </button>
                    </td>
                  </tr>
                {/each}
                {#if getAllModelPermissions().length === 0}
                  <tr><td colspan="4" class="px-4 py-10 text-center text-[13px] text-brand-subtle">No permissions found</td></tr>
                {/if}
              </tbody>
            </table>
          </div>
        {/if}
      </main>
    </div>
  </div>
{/if}

<!-- Toast -->
{#if toast}
  <div class="fixed bottom-6 right-6 flex items-center gap-2 px-4 py-2.5 rounded-md text-[13px] font-medium z-[1000] shadow-[0_8px_24px_rgba(0,0,0,0.4)] backdrop-blur-md animate-[slideIn_0.3s_ease] {toast.type === 'success' ? 'bg-green-dim border border-green-500/20 text-green-300' : 'bg-red-dim border border-red-500/20 text-red-300'}">
    {#if toast.type === 'success'}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
    {:else}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
    {/if}
    {toast.message}
  </div>
{/if}

<!-- Modal -->
{#if modal}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-[500] animate-[fadeIn_0.15s_ease] p-4" onclick={() => { if (!modalLoading) modal = null; }}>
    <div class="w-full max-w-[400px] p-7 bg-brand-surface border border-white/5 rounded-xl shadow-[0_16px_48px_rgba(0,0,0,0.5)] animate-[cardUp_0.3s_ease]" onclick={(e) => e.stopPropagation()}>

      <!-- Content wrapper blocks using same style -->
      {#if modal.type === 'createUser'}
        <h3 class="text-base font-bold tracking-tight text-white mb-5">Create User</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitCreateUser(); }}>
          <div class="mb-4">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Username</label>
            <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="text" bind:value={modalForm.username} required disabled={modalLoading} />
          </div>
          <div class="mb-5">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Password</label>
            <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="password" bind:value={modalForm.password} required disabled={modalLoading} />
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-4 py-2 rounded text-[12px] font-semibold text-brand-muted border border-white/5 hover:text-white hover:border-white/10 transition-colors" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="px-4 py-2 rounded text-[12px] font-semibold bg-accent text-brand-bg w-20 flex justify-center hover:brightness-110 transition-all disabled:opacity-50" disabled={modalLoading}>{#if modalLoading}<span class="w-4 h-4 rounded-full border-2 border-brand-bg/30 border-t-brand-bg animate-spin"></span>{:else}Create{/if}</button>
          </div>
        </form>

      {:else if modal.type === 'searchUser'}
        <h3 class="text-base font-bold tracking-tight text-white mb-5">Find User</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitSearchUser(); }}>
          <div class="mb-5">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Username</label>
            <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="text" bind:value={modalForm.username} required disabled={modalLoading} />
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-4 py-2 rounded text-[12px] font-semibold text-brand-muted border border-white/5 hover:text-white hover:border-white/10 transition-colors" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="px-4 py-2 rounded text-[12px] font-semibold bg-accent text-brand-bg w-20 flex justify-center hover:brightness-110 transition-all disabled:opacity-50" disabled={modalLoading}>{#if modalLoading}<span class="w-4 h-4 rounded-full border-2 border-brand-bg/30 border-t-brand-bg animate-spin"></span>{:else}Search{/if}</button>
          </div>
        </form>

      {:else if modal.type === 'resetPassword'}
        <h3 class="text-base font-bold tracking-tight text-white mb-5">Reset Password — {modal.data.username}</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitResetPassword(); }}>
          <div class="mb-5">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">New Password</label>
            <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="password" bind:value={modalForm.password} required disabled={modalLoading} />
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-4 py-2 rounded text-[12px] font-semibold text-brand-muted border border-white/5 hover:text-white hover:border-white/10 transition-colors" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="px-4 py-2 rounded text-[12px] font-semibold bg-accent text-brand-bg w-20 flex justify-center hover:brightness-110 transition-all disabled:opacity-50" disabled={modalLoading}>{#if modalLoading}<span class="w-4 h-4 rounded-full border-2 border-brand-bg/30 border-t-brand-bg animate-spin"></span>{:else}Update{/if}</button>
          </div>
        </form>

      {:else if modal.type === 'confirmDelete'}
        <h3 class="text-base font-bold tracking-tight text-white mb-4">Delete User</h3>
        <p class="text-[13px] text-brand-muted mb-6 leading-relaxed">Are you sure you want to delete <strong class="text-white">{modal.data.username}</strong>?</p>
        <div class="flex justify-end gap-2">
          <button class="px-4 py-2 rounded text-[12px] font-semibold text-brand-muted border border-white/5 hover:text-white hover:border-white/10 transition-colors" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
          <button class="px-4 py-2 rounded text-[12px] font-semibold bg-red-500 text-white w-20 flex justify-center hover:brightness-110 transition-all disabled:opacity-50" onclick={submitDeleteUser} disabled={modalLoading}>{#if modalLoading}<span class="w-4 h-4 rounded-full border-2 border-white/30 border-t-white animate-spin"></span>{:else}Delete{/if}</button>
        </div>

      {:else if modal.type === 'grantExpPerm'}
        <h3 class="text-base font-bold tracking-tight text-white mb-5">Grant Experiment Permission</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitGrantExpPerm(); }}>
          <div class="mb-4">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Username</label>
            <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="text" bind:value={modalForm.username} required disabled={modalLoading} />
          </div>
          <div class="mb-4">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Experiment</label>
            {#if experiments.length > 0}
              <select class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" bind:value={modalForm.experiment_id} required disabled={modalLoading}>
                <option value="">Select...</option>
                {#each experiments as exp}<option value={exp.experiment_id}>{exp.name} (#{exp.experiment_id})</option>{/each}
              </select>
            {:else}
              <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="text" bind:value={modalForm.experiment_id} required disabled={modalLoading} />
            {/if}
          </div>
          <div class="mb-5">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Permission</label>
            <select class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" bind:value={modalForm.permission} required disabled={modalLoading}>
              {#each permissionLevels as level}<option value={level}>{level}</option>{/each}
            </select>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-4 py-2 rounded text-[12px] font-semibold text-brand-muted border border-white/5 hover:text-white hover:border-white/10 transition-colors" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="px-4 py-2 rounded text-[12px] font-semibold bg-accent text-brand-bg w-20 flex justify-center hover:brightness-110 transition-all disabled:opacity-50" disabled={modalLoading}>{#if modalLoading}<span class="w-4 h-4 rounded-full border-2 border-brand-bg/30 border-t-brand-bg animate-spin"></span>{:else}Grant{/if}</button>
          </div>
        </form>

      {:else if modal.type === 'grantModelPerm'}
        <h3 class="text-base font-bold tracking-tight text-white mb-5">Grant Model Permission</h3>
        <form onsubmit={(e) => { e.preventDefault(); submitGrantModelPerm(); }}>
          <div class="mb-4">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Username</label>
            <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="text" bind:value={modalForm.username} required disabled={modalLoading} />
          </div>
          <div class="mb-4">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Model</label>
            {#if models.length > 0}
              <select class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" bind:value={modalForm.name} required disabled={modalLoading}>
                <option value="">Select...</option>
                {#each models as m}<option value={m.name}>{m.name}</option>{/each}
              </select>
            {:else}
              <input class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" type="text" bind:value={modalForm.name} required disabled={modalLoading} />
            {/if}
          </div>
          <div class="mb-5">
            <label class="block text-[12px] font-medium text-brand-muted mb-1.5">Permission</label>
            <select class="w-full px-3 py-2 bg-brand-elevated border border-white/5 rounded text-[14px] text-white outline-none focus:border-accent" bind:value={modalForm.permission} required disabled={modalLoading}>
              {#each permissionLevels as level}<option value={level}>{level}</option>{/each}
            </select>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-4 py-2 rounded text-[12px] font-semibold text-brand-muted border border-white/5 hover:text-white hover:border-white/10 transition-colors" onclick={() => modal = null} disabled={modalLoading}>Cancel</button>
            <button type="submit" class="px-4 py-2 rounded text-[12px] font-semibold bg-accent text-brand-bg w-20 flex justify-center hover:brightness-110 transition-all disabled:opacity-50" disabled={modalLoading}>{#if modalLoading}<span class="w-4 h-4 rounded-full border-2 border-brand-bg/30 border-t-brand-bg animate-spin"></span>{:else}Grant{/if}</button>
          </div>
        </form>
      {/if}
    </div>
  </div>
{/if}
