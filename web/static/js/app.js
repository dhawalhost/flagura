/**
 * Flagura Control Plane Application Bundle
 * Modernized & CSP-Hardened Static JavaScript
 */

// ==========================================
// 1. Core Utilities & Hashing
// ==========================================

// FNV-1a 64-bit Hash helper for sticky deterministic bucketing
function getStickyBucketJs(key, salt = '') {
	const FNV_OFFSET = 0xcbf29ce484222325n;
	const FNV_PRIME = 0x100000001b3n;
	let hash = FNV_OFFSET;
	const str = key + ':' + salt;
	for (let i = 0; i < str.length; i++) {
		hash = hash ^ BigInt(str.charCodeAt(i));
		hash = (hash * FNV_PRIME) & 0xffffffffffffffffn;
	}
	const bucket = Number((hash % 10000n)) / 100;
	return { bucket: Number(bucket.toFixed(2)), hashRaw: hash.toString(16).padStart(16, '0') };
}

// Global Toast Notification Dispatcher
let globalToastHandler = null;
function showToast(msg, type = 'info') {
	if (globalToastHandler) {
		globalToastHandler(msg, type);
	}
}
window.showToast = showToast;

// Global Modal Helper Triggers
window.openExperimentModal = function(flagKey) {
	window.dispatchEvent(new CustomEvent('open-experiment-modal', { detail: { key: flagKey } }));
};

window.openHygieneModal = function(data) {
	window.dispatchEvent(new CustomEvent('open-hygiene-modal', { detail: data }));
};

// ==========================================
// 2. Alpine.js Component Registrations
// ==========================================

document.addEventListener('alpine:init', () => {
	// ==========================================
	// Micro-Components for CSP Compliance
	// ==========================================

	// Dropdown Micro-Component
	function dropdownComponent() {
		return {
			isOpen: false,
			toggle() { this.isOpen = !this.isOpen; },
			close() { this.isOpen = false; },
			open() { this.isOpen = true; },
			selectEnv(env) {
				if (window.FlaguraApp && typeof window.FlaguraApp.switchEnv === 'function') {
					window.FlaguraApp.switchEnv(env);
				}
				this.close();
			},
			openProfile() {
				if (window.FlaguraApp && typeof window.FlaguraApp.navigateTo === 'function') {
					window.FlaguraApp.navigateTo('profile');
				}
				this.close();
			},
			selectProject(el) {
				if (window.FlaguraApp && typeof window.FlaguraApp.switchProject === 'function') {
					window.FlaguraApp.switchProject(el);
				}
				this.close();
			},
			openCreateProject() {
				this.close();
				window.dispatchEvent(new CustomEvent('open-create-project-modal'));
			},
			openInvite() {
				this.close();
				window.dispatchEvent(new CustomEvent('open-invite-modal'));
			}
		};
	}
	Alpine.data('dropdownComponent', dropdownComponent);
	window.dropdownComponent = dropdownComponent;

	// Toggle Micro-Component
	function toggleComponent(initialState = false) {
		return {
			isToggled: initialState,
			toggle() { this.isToggled = !this.isToggled; },
			on() { this.isToggled = true; },
			off() { this.isToggled = false; }
		};
	}
	Alpine.data('toggleComponent', toggleComponent);
	window.toggleComponent = toggleComponent;

	// Tab Micro-Component
	function tabComponent(initialTab = '') {
		return {
			currentTab: initialTab,
			setTab(tab) { this.currentTab = tab; },
			handleTabClick(el) {
				const tab = el.dataset.tab;
				if (tab) this.setTab(tab);
			}
		};
	}
	Alpine.data('tabComponent', tabComponent);
	window.tabComponent = tabComponent;

	// Landing Install CLI Component
	function installCliComponent() {
		return {
			installTab: 'curl',
			setTab(tab) { this.installTab = tab; },
			copyCommand() {
				const cmdMap = {
					curl: 'curl -sSL https://flagura.dev/install.sh | bash',
					go: 'go install github.com/dhawalhost/flagura/cmd/cli@latest',
					docker: 'docker run --rm -it ghcr.io/dhawalhost/flagura:latest flagura --help'
				};
				const cmd = cmdMap[this.installTab] || cmdMap.curl;
				navigator.clipboard.writeText(cmd);
				showToast('Copied ' + this.installTab + ' command!');
			}
		};
	}
	Alpine.data('installCliComponent', installCliComponent);
	window.installCliComponent = installCliComponent;

	// Feature Flag Table Row Component (Zero-Eval CSP Compliant)
	function flagRowComponent() {
		return {
			key: '',
			envs: {},
			copied: false,
			init() {
				if (this.$el) {
					this.key = this.$el.dataset.key || '';
					try {
						this.envs = JSON.parse(this.$el.dataset.envs || '{}');
					} catch (e) {
						this.envs = {};
					}
				}
			},
			get isDev() { return !!(this.envs && this.envs.development && this.envs.development.enabled); },
			get isStg() { return !!(this.envs && this.envs.staging && this.envs.staging.enabled); },
			get isProd() { return !!(this.envs && this.envs.production && this.envs.production.enabled); },
			get prodPct() { return (this.envs && this.envs.production && this.envs.production.percentage !== undefined) ? this.envs.production.percentage : 0; },
			get devPct() { return (this.envs && this.envs.development && this.envs.development.percentage !== undefined) ? this.envs.development.percentage : 0; },
			get stgPct() { return (this.envs && this.envs.staging && this.envs.staging.percentage !== undefined) ? this.envs.staging.percentage : 0; },
			copyKey() {
				const flagKey = this.key || (this.$el && this.$el.dataset.key);
				if (!flagKey) return;
				navigator.clipboard.writeText(flagKey);
				this.copied = true;
				setTimeout(() => this.copied = false, 1500);
			},
			async toggleEnv(env) {
				const flagKey = this.key || (this.$el && this.$el.dataset.key);
				if (!this.envs[env]) this.envs[env] = {};
				const prev = !!this.envs[env].enabled;
				const next = !prev;
				this.envs[env].enabled = next;

				let ok = false;
				if (window.FlaguraApp && typeof window.FlaguraApp.toggleFlagEnvStatus === 'function') {
					ok = await window.FlaguraApp.toggleFlagEnvStatus(flagKey, env, next);
				} else {
					try {
						const res = await fetch('/api/v1/flags/' + flagKey + '/toggle', {
							method: 'PATCH',
							headers: { 'Content-Type': 'application/json' },
							credentials: 'same-origin',
							body: JSON.stringify({ environment: env, enabled: next })
						});
						if (res.ok) {
							if (window.showToast) window.showToast('Flag ' + flagKey + ' [' + env + '] ' + (next ? 'ENABLED' : 'DISABLED'));
							ok = true;
						} else {
							if (res.status === 401) {
								if (window.showToast) window.showToast('Session expired. Redirecting to login...', 'error');
								setTimeout(() => { window.location.href = '/auth'; }, 1000);
							} else if (res.status === 404) {
								if (window.showToast) window.showToast('Flag "' + flagKey + '" not found', 'error');
							} else {
								if (window.showToast) window.showToast('Toggle failed: HTTP ' + res.status, 'error');
							}
						}
					} catch(e) {
						if (window.showToast) window.showToast('Toggle failed: ' + e.message, 'error');
					}
				}
				if (!ok) {
					this.envs[env].enabled = prev;
				}
			},
			toggleDev() { this.toggleEnv('development'); },
			toggleStg() { this.toggleEnv('staging'); },
			toggleProd() { this.toggleEnv('production'); },
			openHygiene(el) {
				const target = el || this.$el;
				if (!target) return;
				window.openHygieneModal({
					key: target.dataset.healthKey || this.key,
					name: target.dataset.healthName || '',
					status: target.dataset.healthStatus || '',
					reason: target.dataset.healthReason || '',
					action: target.dataset.healthAction || ''
				});
			},
			openEditFlag(el) {
				const flagKey = (el && el.dataset.key) || this.key;
				if (window.FlaguraApp && typeof window.FlaguraApp.openEditFlagEditor === 'function') {
					window.FlaguraApp.openEditFlagEditor(flagKey);
				} else {
					window.dispatchEvent(new CustomEvent('open-edit-flag-editor', { detail: { key: flagKey } }));
				}
			},
			openExperiment(el) {
				const flagKey = (el && el.dataset.key) || this.key;
				window.openExperimentModal(flagKey);
			},
			deleteFlag(el) {
				const flagKey = (el && el.dataset.key) || this.key;
				if (window.FlaguraApp && typeof window.FlaguraApp.deleteFlagRow === 'function') {
					window.FlaguraApp.deleteFlagRow(flagKey);
				}
			}
		};
	}
	Alpine.data('flagRowComponent', flagRowComponent);
	window.flagRowComponent = flagRowComponent;

	// Workspace & Project Management Modal Component
	function projectModalComponent() {
		return {
			open: false,
			tab: 'project',
			orgId: '',
			name: '',
			slug: '',
			description: '',
			orgName: '',
			orgSlug: '',
			orgDescription: '',
			inviteEmail: '',
			inviteRole: 'developer',
			inviteUrl: '',
			inviteCopied: false,
			submitting: false,
			errorMessage: '',
			successMessage: '',
			init() {
				if (this.orgId === '' && this.$el && this.$el.querySelector('select[name=orgId] option')) {
					this.orgId = this.$el.querySelector('select[name=orgId] option').value;
				}
			},
			handleOpenProjectModal() {
				this.open = true;
				this.tab = 'project';
				this.errorMessage = '';
				this.successMessage = '';
				this.name = '';
				this.slug = '';
			},
			handleOpenInviteModal() {
				this.open = true;
				this.tab = 'invite';
				this.errorMessage = '';
				this.successMessage = '';
			},
			close() {
				this.open = false;
			},
			setModalTab(t) {
				this.tab = t;
				this.errorMessage = '';
				this.successMessage = '';
			},
			autoSlug() {
				if (!this.slug || this.slug === this.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').slice(0, -1)) {
					this.slug = this.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
				}
			},
			autoOrgSlug() {
				if (!this.orgSlug || this.orgSlug === this.orgName.toLowerCase().replace(/[^a-z0-9]+/g, '-').slice(0, -1)) {
					this.orgSlug = this.orgName.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
				}
			},
			async createProject() {
				if (!this.name.trim()) {
					this.errorMessage = 'Project name is required';
					return;
				}
				this.submitting = true;
				this.errorMessage = '';
				this.successMessage = '';
				try {
					const res = await fetch('/api/v1/projects', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							organization_id: this.orgId,
							name: this.name.trim(),
							slug: this.slug.trim(),
							description: this.description.trim()
						})
					});
					const data = await res.json();
					if (!res.ok) throw new Error(data.error || 'Failed to create project');
					
					this.successMessage = 'Project created successfully!';
					setTimeout(() => {
						window.location.reload();
					}, 600);
				} catch (err) {
					this.errorMessage = err.message || 'Network error';
					this.submitting = false;
				}
			},
			async createOrganization() {
				if (!this.orgName.trim()) {
					this.errorMessage = 'Workspace name is required';
					return;
				}
				this.submitting = true;
				this.errorMessage = '';
				this.successMessage = '';
				try {
					const res = await fetch('/api/v1/organizations', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							name: this.orgName.trim(),
							slug: this.orgSlug.trim(),
							description: this.orgDescription.trim()
						})
					});
					const data = await res.json();
					if (!res.ok) throw new Error(data.error || 'Failed to create organization');
					
					this.successMessage = 'Workspace created successfully!';
					setTimeout(() => {
						window.location.reload();
					}, 600);
				} catch (err) {
					this.errorMessage = err.message || 'Network error';
					this.submitting = false;
				}
			},
			async createInvitation() {
				if (!this.inviteEmail.trim()) {
					this.errorMessage = 'Colleague email is required';
					return;
				}
				this.submitting = true;
				this.errorMessage = '';
				this.successMessage = '';
				try {
					const res = await fetch('/api/v1/invitations', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							organization_id: this.orgId,
							email: this.inviteEmail.trim(),
							role: this.inviteRole
						})
					});
					const data = await res.json();
					if (!res.ok) throw new Error(data.error || 'Failed to create invitation');
					
					this.inviteUrl = window.location.origin + data.invite_url;
					this.successMessage = 'Invitation link created successfully!';
					this.submitting = false;
				} catch (err) {
					this.errorMessage = err.message || 'Network error';
					this.submitting = false;
				}
			},
			copyInviteLink() {
				if (!this.inviteUrl) return;
				navigator.clipboard.writeText(this.inviteUrl);
				this.inviteCopied = true;
				setTimeout(() => this.inviteCopied = false, 2500);
			}
		};
	}
	Alpine.data('projectModalComponent', projectModalComponent);
	window.projectModalComponent = projectModalComponent;

	// 4-Eyes Governance Modal Component
	function governanceModalComponent() {
		return {
			open: false,
			activeTab: 'pending',
			selectedCR: null,
			reviewAction: '',
			reviewComments: '',
			submitting: false,
			toastMsg: '',
			selectCR(cr) {
				this.selectedCR = cr;
				this.reviewAction = '';
				this.reviewComments = '';
			},
			close() {
				this.open = false;
			},
			handleReviewClick(event) {
				const btn = event.currentTarget || event.target;
				const crId = btn.dataset.crId;
				const approved = btn.dataset.approved === 'true';
				if (crId) {
					this.submitReview(crId, approved);
				}
			},
			async submitReview(crId, approved) {
				if (!this.reviewComments && !approved) {
					alert('Please provide rejection comments.');
					return;
				}
				this.submitting = true;
				try {
					const res = await fetch(`/api/v1/change-requests/${crId}/review`, {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							approved: approved,
							comments: this.reviewComments || (approved ? 'Approved for deployment' : 'Rejected')
						})
					});
					const data = await res.json();
					if (res.ok) {
						if (approved) {
							await fetch(`/api/v1/change-requests/${crId}/apply`, { method: 'POST' });
						}
						window.location.reload();
					} else {
						alert(data.error || 'Failed to process change request review');
					}
				} catch (e) {
					alert('Network error submitting review');
				} finally {
					this.submitting = false;
				}
			}
		};
	}
	Alpine.data('governanceModalComponent', governanceModalComponent);
	window.governanceModalComponent = governanceModalComponent;

	// Resolve active dashboard tab from URL pathname or search query
	function resolveInitialView() {
		try {
			const path = (window.location.pathname || '').toLowerCase();
			const params = new URLSearchParams(window.location.search || '');
			const tab = (params.get('tab') || params.get('view') || '').toLowerCase().trim();
			const hash = (window.location.hash || '').replace('#', '').toLowerCase().trim();

			const candidate = tab || hash || (path.startsWith('/dashboard/') ? path.replace(/^\/dashboard\/?/, '').split('/')[0] : '');
			const map = {
				'overview': 'overview',
				'home': 'overview',
				'dashboard': 'overview',
				'flags': 'flags',
				'matrix': 'flags',
				'rollouts': 'flags',
				'analytics': 'analytics',
				'telemetry': 'analytics',
				'stats': 'analytics',
				'evaluator': 'evaluator',
				'sandbox': 'evaluator',
				'live': 'evaluator',
				'benchmark': 'benchmark',
				'latency': 'benchmark',
				'perf': 'benchmark',
				'audit': 'audit',
				'logs': 'audit',
				'trail': 'audit',
				'sdk': 'sdk',
				'apikeys': 'sdk',
				'quickstart': 'sdk',
				'profile': 'profile',
				'settings': 'profile',
				'account': 'profile',
				'editor': 'editor',
				'new': 'editor'
			};
			return map[candidate] || 'overview';
		} catch (e) {
			return 'overview';
		}
	}

	// Global App Root State
	Alpine.data('globalApp', () => ({
		toasts: [],
		currentEnv: 'production',
		activeView: resolveInitialView(),
		previousView: 'overview',
		searchQuery: '',
		isMobileSidebarOpen: false,
		selectedWallpaper: 'https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?auto=format&fit=crop&w=2560&q=85',
		wallpapers: [
			{ id: 'slate-waves', name: 'Sober Slate Waves', url: 'https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?auto=format&fit=crop&w=2560&q=85' },
			{ id: 'modern-architecture', name: 'Architectural Monolith', url: 'https://images.unsplash.com/photo-1486406146926-c627a92ad1ab?auto=format&fit=crop&w=2560&q=85' },
			{ id: 'nordic-mist', name: 'Nordic Mist Horizon', url: 'https://images.unsplash.com/photo-1507525428034-b723cf961d3e?auto=format&fit=crop&w=2560&q=85' },
			{ id: 'charcoal-dunes', name: 'Charcoal Minimal Curves', url: 'https://images.unsplash.com/photo-1550684848-fac1c5b4e853?auto=format&fit=crop&w=2560&q=85' },
			{ id: 'warm-stone', name: 'Warm Stone Dunes', url: 'https://images.unsplash.com/photo-1509316975850-ff9c5deb0cd9?auto=format&fit=crop&w=2560&q=85' }
		],
		currentUser: { id: 'dev-1', name: 'Dhawal (Developer)', email: 'dhawal@flagura.dev', role: 'developer' },
		navigateTo(view) {
			if (!view) return;
			this.previousView = this.activeView;
			this.activeView = view;
			this.isMobileSidebarOpen = false;
			this.syncUrl(view);
		},
		syncUrl(view) {
			try {
				if (window.history && window.history.pushState) {
					const url = new URL(window.location.href);
					url.pathname = '/dashboard';
					if (url.searchParams.get('tab') !== view) {
						url.searchParams.set('tab', view);
						window.history.pushState({ tab: view }, '', url.toString());
					}
				}
			} catch(e) {}
		},
		openGovernanceModal() {
			this.isMobileSidebarOpen = false;
			window.dispatchEvent(new CustomEvent('open-governance-modal'));
		},
		focusSearch(inputEl) {
			if (document.activeElement && document.activeElement.tagName !== 'INPUT') {
				if (inputEl && typeof inputEl.focus === 'function') inputEl.focus();
			}
		},
		switchEnv(env) {
			this.currentEnv = env;
			this.showToast('Switched to ' + env.charAt(0).toUpperCase() + env.slice(1));
		},
		showToast(msg, type = 'info') {
			const id = Date.now();
			this.toasts.push({ id, message: msg, type });
			setTimeout(() => {
				this.toasts = this.toasts.filter(t => t.id !== id);
			}, 3500);
		},
		getViewTitle() {
			const titles = {
				overview: 'Overview',
				flags: 'Flags & Rollouts',
				analytics: 'Analytics & Telemetry',
				evaluator: 'Live Evaluator',
				benchmark: 'Latency Benchmark',
				audit: 'Audit Trail',
				sdk: 'SDK Integration',
				editor: 'Flag Editor',
				profile: 'Profile Settings'
			};
			return titles[this.activeView] || 'Console';
		},
		async switchProject(projectID) {
			if (projectID && projectID.dataset && projectID.dataset.projectId) {
				projectID = projectID.dataset.projectId;
			}
			try {
				const res = await fetch('/api/v1/projects/active', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ project_id: projectID })
				});
				if (res.ok) {
					window.location.reload();
				} else {
					this.showToast('Failed to switch project', 'error');
				}
			} catch (e) {
				this.showToast('Network error switching project', 'error');
			}
		},
		openNewFlagEditor() {
			this.previousView = this.activeView;
			this.$dispatch('populate-editor', { isEditing: false, currentEnv: this.currentEnv });
			this.activeView = 'editor';
			this.syncUrl('editor');
		},
		openEditFlagEditor(key) {
			this.previousView = this.activeView;
			this.$dispatch('populate-editor', { isEditing: true, key, currentEnv: this.currentEnv });
			this.activeView = 'editor';
			this.syncUrl('editor');
		},
		openEvaluatorForFlag(flagKey) {
			if (flagKey && flagKey.dataset && flagKey.dataset.key) {
				flagKey = flagKey.dataset.key;
			}
			this.$dispatch('select-evaluator-flag', flagKey);
			this.activeView = 'evaluator';
			this.syncUrl('evaluator');
		},
		async toggleFlagEnvStatus(key, env, isEnabled) {
			try {
				const res = await fetch('/api/v1/flags/' + key + '/toggle', {
					method: 'PATCH',
					headers: { 'Content-Type': 'application/json' },
					credentials: 'same-origin',
					body: JSON.stringify({ environment: env, enabled: isEnabled, actor: this.currentUser ? this.currentUser.email : 'admin@flagura.dev' })
				});
				if (res.ok) {
					this.showToast('Flag ' + key + ' [' + env + '] ' + (isEnabled ? 'ENABLED' : 'DISABLED'));
					return true;
				} else {
					if (res.status === 401) {
						this.showToast('Session expired. Redirecting to login...', 'error');
						setTimeout(() => { window.location.href = '/auth'; }, 1000);
					} else if (res.status === 404) {
						this.showToast('Flag "' + key + '" not found', 'error');
					} else {
						this.showToast('Toggle failed: server error (HTTP ' + res.status + ')', 'error');
					}
					return false;
				}
			} catch(e) {
				this.showToast('Toggle failed: network error', 'error');
				return false;
			}
		},
		async toggleFlagStatus(key, isEnabled) {
			try {
				const res = await fetch('/api/v1/flags/' + key + '/toggle', {
					method: 'PATCH',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ environment: this.currentEnv, enabled: isEnabled, actor: this.currentUser ? this.currentUser.email : 'admin@flagura.dev' })
				});
				if (res.ok) {
					this.showToast('Flag ' + key + ' ' + (isEnabled ? 'ENABLED' : 'DISABLED (Kill-Switch)'));
				}
			} catch(e) {
				this.showToast('Toggle failed', 'error');
			}
		},
		updateRollout(key, pct) {
			fetch('/api/v1/flags/' + key + '/rollout', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ environment: this.currentEnv, percentage: Number(pct), actor: this.currentUser ? this.currentUser.email : 'admin@flagura.dev' })
			}).then(() => {
				this.showToast('Rollout for ' + key + ' set to ' + pct + '%');
			});
		},
		async deleteFlagRow(key) {
			if (confirm('Permanently remove feature flag ' + key + '?')) {
				const res = await fetch('/api/v1/flags/' + key, { method: 'DELETE' });
				if (res.ok) {
					const row = document.getElementById('flag-row-' + key);
					if (row) row.remove();
					const card = document.getElementById('flag-card-' + key);
					if (card) card.remove();
					this.showToast('Deleted flag ' + key);
				}
			}
		},
		async logout() {
			try {
				await fetch('/api/v1/auth/logout', { method: 'POST' });
				this.showToast('Logged out');
				setTimeout(() => {
					window.location.href = '/auth';
				}, 300);
			} catch(e) {
				window.location.href = '/auth';
			}
		},
		async checkAuth() {
			if (window.location.pathname.startsWith('/dashboard')) {
				try {
					const res = await fetch('/api/v1/auth/me', { credentials: 'same-origin' });
					if (res.ok) {
						const u = await res.json();
						this.currentUser = u;
					} else if (res.status === 401) {
						window.location.href = '/auth?redirect=' + encodeURIComponent(window.location.pathname + window.location.search);
					}
				} catch(e) {}
			}
		},
		init() {
			window.FlaguraApp = this;
			globalToastHandler = (msg, type = 'info') => this.showToast(msg, type);
			window.showToast = globalToastHandler;

			window.addEventListener('popstate', () => {
				const v = resolveInitialView();
				if (v && this.activeView !== v) {
					this.activeView = v;
				}
			});

			try {
				const params = new URLSearchParams(window.location.search || '');
				const envParam = (params.get('env') || '').toLowerCase().trim();
				if (envParam && ['production', 'staging', 'development'].includes(envParam)) {
					this.currentEnv = envParam;
				}
				const searchParam = params.get('search');
				if (searchParam) {
					this.searchQuery = searchParam;
				}
				const modalParam = (params.get('tab') || params.get('modal') || '').toLowerCase().trim();
				if (modalParam === 'governance' || modalParam === 'approvals') {
					this.$nextTick(() => window.dispatchEvent(new CustomEvent('open-governance-modal')));
				} else if (modalParam === 'hygiene' || modalParam === 'cleanup') {
					this.$nextTick(() => window.dispatchEvent(new CustomEvent('open-hygiene-modal', { detail: {} })));
				} else if (modalParam === 'project' || modalParam === 'new-project') {
					this.$nextTick(() => window.dispatchEvent(new CustomEvent('open-create-project-modal')));
				} else if (modalParam === 'invite') {
					this.$nextTick(() => window.dispatchEvent(new CustomEvent('open-invite-modal')));
				}
			} catch(e) {}

			this.checkAuth();
		}
	}));

	// Flag Matrix Enterprise Component
	Alpine.data('flagMatrixEnterpriseComponent', () => ({
		searchQuery: '',
		envFilter: 'all',
		selectedTag: 'all',
		healthFilter: 'all',
		selectedFlags: [],
		toggleSelectAll(checked) {
			if (checked) {
				const rows = document.querySelectorAll('[id^="flag-row-"]');
				this.selectedFlags = Array.from(rows).map(r => r.id.replace('flag-row-', ''));
			} else {
				this.selectedFlags = [];
			}
		},
		async bulkToggleEnv(env, enabled) {
			for (const key of this.selectedFlags) {
				await this.toggleFlagEnvStatus(key, env, enabled);
			}
			this.showToast('Updated ' + this.selectedFlags.length + ' flags in ' + env);
			setTimeout(() => window.location.reload(), 600);
		},
		async bulkDelete() {
			if (confirm('Permanently delete ' + this.selectedFlags.length + ' selected flags?')) {
				for (const key of this.selectedFlags) {
					await fetch('/api/v1/flags/' + key, { method: 'DELETE' });
				}
				this.showToast('Deleted ' + this.selectedFlags.length + ' flags');
				setTimeout(() => window.location.reload(), 500);
			}
		},
		matchesEnterpriseFilter(key, name, tags, isProd, prodPct, healthStatus) {
			if (this.envFilter === 'prod_enabled' && !isProd) return false;
			if (this.envFilter === 'prod_disabled' && isProd) return false;
			if (this.envFilter === 'canary' && (!isProd || prodPct >= 100)) return false;

			if (this.selectedTag !== 'all' && !tags.includes(this.selectedTag)) return false;

			// Health Status Filter: 'all', 'stale', 'dead', 'active'
			if (this.healthFilter === 'stale' && healthStatus !== 'READY_FOR_CLEANUP') return false;
			if (this.healthFilter === 'dead' && healthStatus !== 'DEAD_FLAG') return false;
			if (this.healthFilter === 'active' && healthStatus !== 'ACTIVE') return false;

			const q = (this.searchQuery || '').toLowerCase().trim();
			if (!q) return true;
			return key.toLowerCase().includes(q) || name.toLowerCase().includes(q) || tags.toLowerCase().includes(q);
		},
		matchesRow(el, isProd, prodPct) {
			if (!el) return true;
			const key = el.dataset.key || '';
			const name = el.dataset.name || '';
			const tags = el.dataset.tags || '';
			const healthStatus = el.dataset.health || '';
			return this.matchesEnterpriseFilter(key, name, tags, isProd, prodPct, healthStatus);
		}
	}));

	// Bento Enterprise Dashboard Component
	Alpine.data('bentoEnterpriseDashboardComponent', () => ({
		activeProdCount: 0,
		staleCount: 0,
		init() {
			this.$nextTick(() => {
				const onToggles = document.querySelectorAll('.bg-emerald-600');
				this.activeProdCount = Math.max(0, onToggles.length - 2);

				// Dynamically count flags ready for cleanup
				const staleBadges = document.querySelectorAll('.bg-amber-50');
				this.staleCount = staleBadges.length;
			});
		}
	}));

	// Flag Matrix View Component
	Alpine.data('flagMatrixViewComponent', () => ({
		statusFilter: 'all',
		selectedTag: 'all',
		searchQuery: '',
		matchesFilter(key, name, tags, isEnabled, rulesCount, strategy) {
			// Status filter
			if (this.statusFilter === 'enabled' && !isEnabled) return false;
			if (this.statusFilter === 'disabled' && isEnabled) return false;
			if (this.statusFilter === 'rollout' && (!isEnabled || strategy !== 'percentage')) return false;
			if (this.statusFilter === 'rules' && (!isEnabled || rulesCount === 0)) return false;

			// Tag filter
			if (this.selectedTag !== 'all' && !tags.includes(this.selectedTag)) return false;

			// Search query
			const q = (this.searchQuery || '').toLowerCase().trim();
			if (!q) return true;
			return key.toLowerCase().includes(q) || name.toLowerCase().includes(q) || tags.toLowerCase().includes(q);
		}
	}));

	// Live Evaluator View Component
	Alpine.data('liveEvaluatorComponent', () => ({
		selectedFlagKey: 'ai-smart-search',
		evaluating: false,
		evalResults: {},
		evalTraces: {},
		copiedCurl: false,
		currentBucket: 50,
		showAdvanced: false,
		openCurl: false,
		context: {
			user_id: 'usr_dhawal_01',
			email: 'dhawal@flagura.dev',
			country: 'US',
			role: 'admin',
			tier: 'enterprise'
		},
		customJson: '{\n  "app_version": "2.4.0",\n  "beta_group": "alpha"\n}',
		init() {
			this.evaluateNow();
			this.$watch('context.user_id', () => {
				const b = getStickyBucketJs(this.context.user_id || 'anon', 'salt');
				this.currentBucket = b.bucket;
			});
			window.addEventListener('select-evaluator-flag', (e) => {
				this.selectedFlagKey = e.detail;
				this.evaluateNow();
			});
		},
		setEnvAndEvaluate(env) {
			this.currentEnv = env;
			this.evaluateNow();
		},
		selectFlagAndEvaluate(key) {
			this.selectedFlagKey = key;
			this.evaluateNow();
		},
		getEvalResultsCount() {
			return Object.keys(this.evalResults || {}).length;
		},
		applyPreset(uid, email, country, role, tier) {
			if (uid && uid.dataset) {
				const el = uid;
				uid = el.dataset.uid;
				email = el.dataset.email;
				country = el.dataset.country;
				role = el.dataset.role;
				tier = el.dataset.tier;
			}
			this.context.user_id = uid;
			this.context.email = email;
			this.context.country = country;
			this.context.role = role;
			this.context.tier = tier;
			this.evaluateNow();
			this.showToast('Applied test persona: ' + uid);
		},
		async evaluateNow() {
			this.evaluating = true;
			try {
				let parsedCustom = {};
				try { if (this.customJson.trim()) parsedCustom = JSON.parse(this.customJson); } catch(e) {}
				const flagsList = this.selectedFlagKey === 'all' ? [] : [this.selectedFlagKey];

				const res = await fetch('/api/v1/evaluate', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					credentials: 'same-origin',
					body: JSON.stringify({
						flags: flagsList,
						trace: true,
						context: { ...this.context, ...parsedCustom, environment: this.currentEnv }
					})
				});
				if (res.ok) {
					const data = await res.json();
					this.evalResults = data.results || {};
					this.evalTraces = data.traces || {};
				} else if (res.status === 401 && window.showToast) {
					window.showToast('Session expired. Please log in.', 'error');
				}
			} catch(e) {
				// handled
			} finally {
				this.evaluating = false;
			}
		},
		getCurlCommand() {
			const flagsArg = this.selectedFlagKey === 'all' ? '["*"]' : `["${this.selectedFlagKey}"]`;
			return `curl -X POST ${window.location.origin}/api/v1/evaluate \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "flags": ${flagsArg},\n    "context": {\n      "user_id": "${this.context.user_id}",\n      "email": "${this.context.email}",\n      "country": "${this.context.country}",\n      "role": "${this.context.role}",\n      "tier": "${this.context.tier}",\n      "environment": "${this.currentEnv}"\n    }\n  }'`;
		},
		copyCurl() {
			navigator.clipboard.writeText(this.getCurlCommand());
			this.copiedCurl = true;
			setTimeout(() => this.copiedCurl = false, 2000);
		},
		// CSP-safe wrapper for Math.min used in :style binding
		clampBucket(v) {
			return Math.min(v, 97);
		}
	}));

	// Analytics View Component
	Alpine.data('analyticsViewComponent', () => ({
		selectedAnalyticsFlag: 'all',
		chart: null,
		totalEvaluations: 0,
		init() {
			this.$nextTick(() => this.renderChart());
		},
		updateAnalyticsChart() {
			this.renderChart();
		},
		async renderChart() {
			const ctx = document.getElementById('analyticsChartCanvas');
			if (!ctx) return;

			let points = [14200, 11500, 9800, 18400, 42100, 78900, 114000, 128500, 134200, 119000, 89400, 48200];
			try {
				const res = await fetch('/api/v1/telemetry/stats?flag=' + encodeURIComponent(this.selectedAnalyticsFlag));
				if (res.ok) {
					const stats = await res.json();
					if (stats.hourly_points && stats.hourly_points.some(p => p > 0)) {
						const hourly = stats.hourly_points;
						const sampled = [];
						for (let i = 0; i < 24; i += 2) {
							sampled.push((hourly[i] || 0) + (hourly[i+1] || 0));
						}
						points = sampled;
					}
					if (stats.total_evaluations !== undefined) {
						this.totalEvaluations = stats.total_evaluations;
					}
				}
			} catch (e) {}

			if (this.chart) this.chart.destroy();

			const hours = ['00:00', '02:00', '04:00', '06:00', '08:00', '10:00', '12:00', '14:00', '16:00', '18:00', '20:00', '22:00'];

			this.chart = new Chart(ctx, {
				type: 'line',
				data: {
					labels: hours,
					datasets: [{
						label: '24h Evaluations Throughput',
						data: points,
						borderColor: '#2563eb',
						backgroundColor: 'rgba(37, 99, 235, 0.08)',
						fill: true,
						tension: 0.35,
						borderWidth: 2.5,
						pointBackgroundColor: '#2563eb',
						pointRadius: 3
					}]
				},
				options: {
					responsive: true,
					maintainAspectRatio: false,
					plugins: { legend: { display: false } },
					scales: {
						x: { grid: { color: 'rgba(0, 0, 0, 0.04)' }, ticks: { color: '#64748b', font: { size: 10 } } },
						y: { grid: { color: 'rgba(0, 0, 0, 0.04)' }, ticks: { color: '#64748b', font: { size: 10 } } }
					}
				}
			});
		}
	}));

	// Benchmark View Component
	Alpine.data('benchmarkViewComponent', () => ({
		iterations: 10000,
		isRunning: false,
		metrics: null,
		chart: null,
		networkProbing: false,
		networkResults: null,
		formatNumber(val) {
			return Number(val || 0).toLocaleString();
		},
		getTargetOrigin() {
			return window.location.origin;
		},
		getSpeedupFactor() {
			if (!this.networkResults || !this.networkResults.avg) return '0x';
			return Math.round(this.networkResults.avg * 1000000 / 85).toLocaleString() + 'x';
		},
		init() {
			this.$nextTick(() => {
				if (!this.metrics) this.runStressTest();
			});
		},
		async runStressTest() {
			this.isRunning = true;
			try {
				const res = await fetch('/api/v1/benchmark', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					credentials: 'same-origin',
					body: JSON.stringify({ iterations: Number(this.iterations), environment: this.currentEnv })
				});
				if (!res.ok) {
					if (res.status === 401 && window.showToast) {
						window.showToast('Session expired. Please log in.', 'error');
					}
					return;
				}
				const data = await res.json();
				this.metrics = data;
				this.$nextTick(() => this.renderHistogram(data.hashBuckets || []));
			} catch(e) {
				if (window.showToast) window.showToast('Benchmark failed: ' + e.message, 'error');
			} finally {
				this.isRunning = false;
			}
		},
		async probeLiveNetwork() {
			this.networkProbing = true;
			const samples = [];
			const payload = JSON.stringify({
				flags: ['*'],
				context: { user_id: 'probe_user_' + Date.now(), environment: this.currentEnv }
			});
			try {
				for (let i = 0; i < 5; i++) {
					const start = performance.now();
					await fetch('/api/v1/evaluate', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: payload
					});
					const end = performance.now();
					samples.push(Math.max(0.1, end - start));
					await new Promise(r => setTimeout(r, 40));
				}
				samples.sort((a, b) => a - b);
				const sum = samples.reduce((acc, v) => acc + v, 0);
				this.networkResults = {
					samples: samples,
					min: samples[0],
					max: samples[samples.length - 1],
					p50: samples[Math.floor(samples.length / 2)],
					avg: sum / samples.length
				};
				this.showToast('Network probe complete: avg ' + this.networkResults.avg.toFixed(1) + 'ms');
			} catch (e) {
				this.showToast('Network probe failed', 'error');
			} finally {
				this.networkProbing = false;
			}
		},
		renderHistogram(buckets) {
			const ctx = document.getElementById('benchmarkHistogramCanvas');
			if (!ctx) return;
			if (this.chart) this.chart.destroy();
			const labels = Array.from({ length: 100 }, (_, i) => i + '%');
			this.chart = new Chart(ctx, {
				type: 'bar',
				data: {
					labels: labels,
					datasets: [{
						data: buckets,
						backgroundColor: 'rgba(37, 99, 235, 0.65)',
						borderColor: '#2563eb',
						borderWidth: 1,
						borderRadius: 2
					}]
				},
				options: {
					responsive: true,
					maintainAspectRatio: false,
					plugins: { legend: { display: false } },
					scales: {
						x: { display: false },
						y: { grid: { color: 'rgba(0, 0, 0, 0.05)' }, ticks: { color: '#64748b', font: { size: 10 } } }
					}
				}
			});
		}
	}));

	// Audit View Component
	Alpine.data('auditViewComponent', () => ({
		loading: false,
		async fetchLogs() {
			this.loading = true;
			try {
				await fetch('/api/v1/audit-logs');
				this.showToast('Audit trail refreshed');
			} finally {
				this.loading = false;
			}
		}
	}));

	// SDK View Component
	Alpine.data('sdkViewComponent', () => ({
		sdkTab: 'apikeys',
		openfeatureLang: 'go',
		copied: false,
		apiKeys: [],
		showKeyModal: false,
		keyForm: { name: '', role: 'developer', environment: 'production' },
		newlyCreatedKey: null,
		keyCopied: false,
		formatDate(dateStr) {
			if (!dateStr) return '';
			try {
				return new Date(dateStr).toLocaleDateString();
			} catch (e) {
				return String(dateStr);
			}
		},
		async fetchAPIKeys() {
			try {
				const res = await fetch('/api/v1/api-keys');
				if (res.ok) {
					const data = await res.json();
					this.apiKeys = data.api_keys || [];
				}
			} catch (e) {
				this.showToast('Could not load API keys', 'error');
			}
		},
		async generateKey() {
			if (!this.keyForm.name.trim()) return;
			try {
				const res = await fetch('/api/v1/api-keys', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(this.keyForm)
				});
				if (res.ok) {
					const data = await res.json();
					this.newlyCreatedKey = data.api_key;
					this.showKeyModal = false;
					this.keyForm = { name: '', role: 'developer', environment: 'production' };
					this.fetchAPIKeys();
				} else {
					const err = await res.text();
					alert('Failed to generate key: ' + err);
				}
			} catch (e) {
				alert('Error generating key: ' + e.message);
			}
		},
		async revokeAPIKey(id) {
			if (!confirm('Are you sure you want to revoke this API key? This action is immediate and cannot be undone.')) return;
			try {
				const res = await fetch('/api/v1/api-keys/' + id, { method: 'DELETE' });
				if (res.ok) {
					this.fetchAPIKeys();
				}
			} catch (e) {
				alert('Error revoking key: ' + e.message);
			}
		},
		copyKeyToken() {
			if (!this.newlyCreatedKey) return;
			navigator.clipboard.writeText(this.newlyCreatedKey.key);
			this.keyCopied = true;
			setTimeout(() => this.keyCopied = false, 2000);
		},
		getSdkSnippet() {
			if (this.sdkTab === 'openfeature') {
				if (this.openfeatureLang === 'go') {
					return `package main\n\nimport (\n    "context"\n    "fmt"\n    flagura "github.com/dhawalhost/flagura/sdks/go"\n    flaguraOF "github.com/dhawalhost/flagura/sdks/go/openfeature"\n    of "github.com/open-feature/go-sdk/openfeature"\n)\n\nfunc main() {\n    // 1. Initialize Flagura in-process evaluation client\n    client := flagura.NewClient("${window.location.origin}", "flg_live_your_api_key",\n        flagura.WithLocalEvaluation(true),\n    )\n    defer client.Close()\n\n    // 2. Register Flagura as the OpenFeature global provider\n    of.SetProvider(flaguraOF.NewProvider(client))\n    ofClient := of.NewClient("backend-service")\n\n    // 3. Evaluate using standard OpenFeature APIs\n    ctx := of.NewEvaluationContext("usr_dhawal_01", map[string]interface{}{\n        "email": "dhawal@flagura.dev",\n        "tier":  "enterprise",\n    })\n    enabled, _ := ofClient.BooleanValue(context.Background(), "ai-smart-search", false, ctx)\n    fmt.Printf("OpenFeature enabled: %v\\n", enabled)\n}`;
				} else if (this.openfeatureLang === 'ts') {
					return `import { FlaguraClient } from '@flagura/sdk';\nimport { FlaguraOpenFeatureProvider } from '@flagura/sdk/openfeature';\nimport { OpenFeature } from '@openfeature/server-sdk';\n\n// 1. Initialize Flagura Client\nconst client = new FlaguraClient({\n  endpoint: '${window.location.origin}',\n  apiKey: 'flg_live_your_api_key'\n});\n\n// 2. Bind Flagura as the OpenFeature Provider\nawait OpenFeature.setProviderAndWait(new FlaguraOpenFeatureProvider(client));\nconst ofClient = OpenFeature.getClient('billing-service');\n\n// 3. Evaluate with standard OpenFeature context\nconst enabled = await ofClient.getBooleanValue('ai-smart-search', false, {\n  targetingKey: 'usr_dhawal_01',\n  email: 'dhawal@flagura.dev',\n  tier: 'enterprise'\n});\n// Feature is active: enabled`;
				} else if (this.openfeatureLang === 'python') {
					return `from openfeature import api\nfrom openfeature.evaluation_context import EvaluationContext\nfrom flagura import FlaguraClient\nfrom flagura.openfeature_provider import FlaguraOpenFeatureProvider\n\n# 1. Initialize client & register OpenFeature provider\nclient = FlaguraClient("${window.location.origin}", api_key="flg_live_your_api_key")\napi.set_provider(FlaguraOpenFeatureProvider(client=client))\nof_client = api.get_client("api-service")\n\n# 2. Evaluate with OpenFeature standard API\ncontext = EvaluationContext(targeting_key="usr_dhawal_01", attributes={"email": "dhawal@flagura.dev", "tier": "enterprise"})\nenabled = of_client.get_boolean_value("ai-smart-search", False, context)\nprint("OpenFeature enabled:", enabled)\nclient.close()`;
				}
			}
			if (this.sdkTab === 'go') {
				return `package main\n\nimport (\n    "context"\n    "fmt"\n    flagura "github.com/dhawalhost/flagura/sdks/go"\n)\n\nfunc main() {\n    // Project scope and environment are automatically resolved from your API key\n    client := flagura.NewClient("${window.location.origin}", "flg_live_your_api_key",\n        flagura.WithLocalEvaluation(true),\n    )\n    defer client.Close()\n\n    res, _ := client.Evaluate(context.Background(), "ai-smart-search", flagura.Context{\n        UserID: "usr_dhawal_01",\n        Email:  "dhawal@flagura.dev",\n    })\n    fmt.Printf("Enabled: %v | Latency: %v ns\\n", res.Enabled, res.EvaluationLatencyNs)\n}`;
			} else if (this.sdkTab === 'ts') {
				return `import { FlaguraClient } from '@flagura/sdk';\n\n// Project scope is automatically resolved from your API key\nconst client = new FlaguraClient({\n  endpoint: '${window.location.origin}',\n  apiKey: 'flg_live_your_api_key'\n});\nconst { enabled, variant } = await client.evaluate('ai-smart-search', {\n  user_id: 'usr_dhawal_01',\n  email: 'dhawal@flagura.dev',\n});\n// Flag resolved: enabled=\${enabled}, variant=\${variant}`;
			} else if (this.sdkTab === 'python') {
				return `from flagura import FlaguraClient, EvaluationContext\n\n# Project scope is automatically resolved from your API key\nclient = FlaguraClient("${window.location.origin}", api_key="flg_live_your_api_key")\nres = client.evaluate("ai-smart-search", EvaluationContext(user_id="usr_dhawal_01", email="dhawal@flagura.dev"))\nprint(res.enabled, res.variant)`;
			} else if (this.sdkTab === 'rust') {
				return `use flagura::{FlaguraClient, EvaluationContext};\n\n#[tokio::main]\nasync fn main() -> Result<(), Box<dyn std::error::Error>> {\n    // Project scope is automatically resolved from your API key\n    let client = FlaguraClient::new("${window.location.origin}")\n        .with_api_key("flg_live_your_api_key");\n    let ctx = EvaluationContext::new("usr_alex_42").with_email("alex@company.com");\n\n    if client.is_enabled("ai-smart-search", &ctx).await {\n        println!("✨ AI Smart Search is active!");\n    }\n    Ok(())\n}`;
			} else if (this.sdkTab === 'cli') {
				const selfHostEnv = window.location.origin !== 'https://flagura.dev' ? `export FLAGURA_ENDPOINT="${window.location.origin}"\n` : '';
				return `# Install or run Flagura Developer CLI\n${selfHostEnv}export FLAGURA_API_KEY="flg_live_your_api_key"\nflagura api-key list\nflagura list\nflagura rollout ai-smart-search 50% --env=production`;
			}
			return `curl -X POST ${window.location.origin}/api/v1/evaluate \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer flg_live_your_api_key" \\\n  -d '{"flags":["ai-smart-search"],"context":{"user_id":"usr_dhawal_01","email":"dhawal@flagura.dev"}}'`;
		},
		copySnippet() {
			navigator.clipboard.writeText(this.getSdkSnippet());
			this.copied = true;
			setTimeout(() => this.copied = false, 2000);
		}
	}));

	// Flag Editor View Component
	Alpine.data('flagEditorComponent', () => ({
		isEditing: false,
		envTab: 'production',
		saving: false,
		form: {
			id: '',
			key: '',
			name: '',
			description: '',
			type: 'boolean',
			tagsInput: 'frontend, checkout',
			environments: {
				production: { enabled: true, strategy: 'percentage', percentage: 50, rules: [] },
				staging: { enabled: true, strategy: 'percentage', percentage: 100, rules: [] },
				development: { enabled: true, strategy: 'boolean', percentage: 100, rules: [] }
			}
		},
		closeEditor() {
			this.navigateTo(this.previousView || 'overview');
		},
		init() {
			window.addEventListener('populate-editor', async (e) => {
				const d = e.detail;
				this.isEditing = d.isEditing;
				if (d.currentEnv) {
					this.envTab = d.currentEnv;
				}
				if (d.isEditing && d.key) {
					try {
						const res = await fetch('/api/v1/flags');
						if (res.ok) {
							const data = await res.json();
							const flag = (data.flags || []).find(f => f.key === d.key || f.id === d.key);
							if (flag) {
								this.form.id = flag.id || flag.key;
								this.form.key = flag.key;
								this.form.name = flag.name;
								this.form.description = flag.description || '';
								this.form.type = flag.type || 'boolean';
								this.form.tagsInput = (flag.tags || []).join(', ');
								this.form.environments = JSON.parse(JSON.stringify(flag.environments || {}));
								for (const env of ['production', 'staging', 'development']) {
									if (!this.form.environments[env]) {
										this.form.environments[env] = { enabled: false, strategy: 'boolean', percentage: 0, rules: [] };
									}
									if (!this.form.environments[env].strategy) {
										this.form.environments[env].strategy = 'boolean';
									}
									if (!this.form.environments[env].rules) {
										this.form.environments[env].rules = [];
									}
									for (const r of this.form.environments[env].rules) {
										if (!r.valuesInput && r.values) {
											r.valuesInput = Array.isArray(r.values) ? r.values.join(', ') : String(r.values);
										}
									}
								}
								return;
							}
						}
					} catch (err) {
						// ignored
					}
				} else {
					this.form.id = 'flag_' + Date.now();
					this.form.key = '';
					this.form.name = '';
					this.form.description = '';
					this.form.type = 'boolean';
					this.form.tagsInput = 'frontend, experimental';
					this.form.environments = {
						production: { enabled: true, strategy: 'percentage', percentage: 50, rules: [] },
						staging: { enabled: true, strategy: 'percentage', percentage: 100, rules: [] },
						development: { enabled: true, strategy: 'boolean', percentage: 100, rules: [] }
					};
				}
			});
		},
		addRule() {
			this.form.environments[this.envTab].rules.push({
				id: 'rule_' + Date.now(),
				name: 'Custom Rule #' + (this.form.environments[this.envTab].rules.length + 1),
				attribute: 'email',
				operator: 'ends_with',
				valuesInput: '@flagura.dev',
				action: 'force_enabled'
			});
		},
		removeRule(idx) {
			this.form.environments[this.envTab].rules.splice(idx, 1);
		},
		syncEnvConfigToAll() {
			const cfg = JSON.parse(JSON.stringify(this.form.environments[this.envTab]));
			this.form.environments.production = JSON.parse(JSON.stringify(cfg));
			this.form.environments.staging = JSON.parse(JSON.stringify(cfg));
			this.form.environments.development = JSON.parse(JSON.stringify(cfg));
			this.showToast('Synchronized ' + this.envTab + ' config to all environments');
		},
		async promoteEnvironment(from, to) {
			if (!confirm(`Are you sure you want to promote ${from} rules directly to ${to}? This will overwrite ${to} configuration.`)) return;
			try {
				const res = await fetch(`/api/v1/flags/${this.form.key}/promote?from=${from}&to=${to}`, { method: 'POST' });
				if (res.ok) {
					this.showToast(`Successfully promoted ${this.form.key} from ${from} to ${to}!`);
					setTimeout(() => window.location.reload(), 700);
				} else {
					const err = await res.text();
					this.showToast(`Promotion failed: ${err}`, 'error');
				}
			} catch (e) {
				this.showToast(`Network error: ${e.message}`, 'error');
			}
		},
		async saveFlag() {
			this.saving = true;
			try {
				const tags = this.form.tagsInput.split(',').map(t => t.trim()).filter(Boolean);
				const envs = JSON.parse(JSON.stringify(this.form.environments));
				for (const envKey of Object.keys(envs)) {
					const envCfg = envs[envKey];
					envCfg.percentage = Number(envCfg.percentage) || 0;
					if (envCfg.rules && Array.isArray(envCfg.rules)) {
						for (const r of envCfg.rules) {
							if (r.valuesInput && typeof r.valuesInput === 'string') {
								r.values = r.valuesInput.split(',').map(v => v.trim()).filter(Boolean);
							}
						}
					}
				}
				const payload = {
					id: this.form.id || this.form.key,
					key: this.form.key,
					name: this.form.name,
					description: this.form.description,
					type: this.form.type,
					tags: tags,
					environments: envs
				};
				const res = await fetch('/api/v1/flags', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(payload)
				});
				if (res.ok) {
					this.showToast('Saved flag ' + this.form.key + ' successfully!');
					setTimeout(() => window.location.reload(), 600);
				} else {
					const err = await res.text();
					this.showToast('Failed to save flag: ' + err, 'error');
				}
			} catch (e) {
				this.showToast('Network error saving flag: ' + e.message, 'error');
			} finally {
				this.saving = false;
			}
		}
	}));

	// Landing Page Component
	Alpine.data('landingPageComponent', () => ({
		consoleMode: 'switchboard',
		testUserId: 'usr_alex_42',
		canaryPercent: 50,
		computedHashHex: '',
		computedBucket: 0,
		isEvaluatedTrue: true,
		heroUserId: 'usr_alex_42',
		heroRolloutPct: 50,
		heroBucket: 0,
		isHeroActive: true,
		sdkLanguage: 'go',
		codeCopied: false,
		benchRunning: false,
		isRunningBenchmark: false,
		benchCount: 100000,
		benchmarkIters: 100000,
		benchDurationMs: '',
		benchmarkTimeMs: '',
		benchOpsPerSec: '',
		benchmarkThroughput: '',
		init() {
			this.computeHashModulo();
			this.runBrowserBenchmark(true);
			this.$nextTick(() => {
				if (window.lucide) window.lucide.createIcons();
			});
		},
		scrollToSection(id) {
			const el = document.getElementById(id);
			if (el) el.scrollIntoView({ behavior: 'smooth' });
		},
		computeHashModulo() {
			const uid = (this.testUserId || 'usr_alex_42').trim();
			const res = getStickyBucketJs(uid, 'ai-smart-search');
			this.computedHashHex = '0x' + res.hashRaw.slice(0, 12).toUpperCase();
			this.computedBucket = Math.floor(res.bucket);
			this.isEvaluatedTrue = this.computedBucket < Number(this.canaryPercent);
			this.heroUserId = uid;
			this.heroRolloutPct = Number(this.canaryPercent);
			this.heroBucket = this.computedBucket;
			this.isHeroActive = this.isEvaluatedTrue;
		},
		updateHeroBucket() {
			this.computeHashModulo();
		},
		selectTestUser(userId) {
			this.testUserId = userId;
			this.computeHashModulo();
		},
		randomizeUser() {
			const users = ['usr_alex_42', 'sarah@stripe.com', 'david_pro_99', 'emma_infra', 'enterprise_vip', 'dev_priya_01', 'cloud_node_88'];
			const current = this.testUserId;
			const pool = users.filter(u => u !== current);
			this.testUserId = pool[Math.floor(Math.random() * pool.length)];
			this.computeHashModulo();
		},
		copySnippet() {
			let code = '';
			if (this.sdkLanguage === 'go') {
				code = 'package main\n\nimport (\n    "context"\n    "fmt"\n    flagura "github.com/dhawalhost/flagura/sdks/go"\n)\n\nfunc main() {\n    client := flagura.NewClient("https://api.flagura.dev", "flg_live_secret", flagura.WithLocalEvaluation(true))\n    defer client.Close()\n    res, _ := client.Evaluate(context.Background(), "ai-smart-search", flagura.Context{UserID: "usr_alex_42", Email: "alex@company.com"})\n    fmt.Printf("Enabled: %v | Latency: %v ns\\n", res.Enabled, res.EvaluationLatencyNs)\n}';
			} else if (this.sdkLanguage === 'ts') {
				code = 'import { FlaguraClient } from \'@flagura/sdk\';\n\nconst client = new FlaguraClient({ endpoint: \'https://api.flagura.dev\', apiKey: \'flg_live_secret\' });\nconst { enabled, variant } = await client.evaluate(\'ai-smart-search\', { user_id: \'usr_alex_42\', email: \'alex@company.com\' });\n// Result: enabled=${enabled}, variant=${variant}';
			} else if (this.sdkLanguage === 'python') {
				code = 'from flagura import FlaguraClient, EvaluationContext\n\nclient = FlaguraClient("https://api.flagura.dev", api_key="flg_live_secret")\nres = client.evaluate("ai-smart-search", EvaluationContext(user_id="usr_alex_42", email="alex@company.com"))\nprint(res.enabled, res.variant)';
			} else if (this.sdkLanguage === 'rust') {
				code = 'use flagura::{FlaguraClient, EvaluationContext};\n\n#[tokio::main]\nasync fn main() -> Result<(), Box<dyn std::error::Error>> {\n    let client = FlaguraClient::new("https://api.flagura.dev").with_api_key("flg_live_secret");\n    let ctx = EvaluationContext::new("usr_alex_42").with_email("alex@company.com");\n    if client.is_enabled("ai-smart-search", &ctx).await {\n        println!("✨ AI Smart Search is active!");\n    }\n    Ok(())\n}';
			} else {
				code = 'curl -X POST https://api.flagura.dev/api/v1/evaluate -H "Content-Type: application/json" -H "Authorization: Bearer flg_live_secret" -d \'{"flags":["ai-smart-search"],"context":{"user_id":"usr_alex_42","email":"alex@company.com"}}\'';
			}
			navigator.clipboard.writeText(code);
			this.codeCopied = true;
			this.showToast('Code snippet copied to clipboard');
			setTimeout(() => this.codeCopied = false, 2000);
		},
		showToast(msg, type = 'info') {
			showToast(msg, type);
		},
		async runBrowserBenchmark(isSilent = false) {
			this.benchRunning = true;
			this.isRunningBenchmark = true;
			if (!isSilent) await new Promise(r => setTimeout(r, 60));
			const iterations = 100000;
			const start = performance.now();
			let hash = 2166136261;
			for (let i = 0; i < iterations; i++) {
				hash ^= (i & 0xff);
				hash = Math.imul(hash, 16777619);
			}
			const end = performance.now();
			const duration = Math.max(0.1, end - start);
			const durStr = duration.toFixed(2);
			const opsSec = Math.round((iterations / (duration / 1000)));
			const opsStr = opsSec.toLocaleString() + ' ops/sec';

			this.benchDurationMs = durStr;
			this.benchmarkTimeMs = durStr;
			this.benchCount = iterations;
			this.benchmarkIters = iterations;
			this.benchOpsPerSec = opsStr;
			this.benchmarkThroughput = opsStr;
			this.benchRunning = false;
			this.isRunningBenchmark = false;
			if (!isSilent) {
				this.showToast('Measured 100,000 evaluations in ' + durStr + 'ms (' + opsStr + ')');
			}
		}
	}));

	// Auth Page Component
	Alpine.data('authPageComponent', () => ({
		mode: 'signin',
		loading: false,
		showPassword: false,
		error: '',
		successMessage: '',
		resetToken: '',
		inviteToken: '',
		inviteOrgName: '',
		loginForm: {
			email: '',
			passphrase: ''
		},
		signUpForm: {
			name: '',
			email: '',
			passphrase: '',
			confirmPassphrase: '',
			role: 'developer'
		},
		forgotForm: {
			email: ''
		},
		resetForm: {
			passphrase: '',
			confirmPassphrase: ''
		},
		criteria: {
			len: false,
			upper: false,
			lower: false,
			digit: false,
			special: false
		},
		passwordStrengthPct: 0,
		passwordStrengthText: '',
		passwordStrengthColor: '',
		passwordStrengthClass: '',

		getTitleText() {
			if (this.mode === 'signup') return 'Create Account';
			if (this.mode === 'forgot') return 'Reset Password';
			if (this.mode === 'reset') return 'Set Password';
			return 'Sign In';
		},

		getSubtitleText() {
			if (this.mode === 'signup') return 'Get started with multi-tenant flag management';
			if (this.mode === 'forgot') return 'We will send you a secure recovery link';
			if (this.mode === 'reset') return 'Choose a strong password to continue';
			return 'Access your team feature flags, rollouts, and environments';
		},

		isMainAuthMode() {
			return this.mode === 'signin' || this.mode === 'signup';
		},

		initAuth() {
			const urlParams = new URLSearchParams(window.location.search);
			const modeParam = urlParams.get('mode');
			const tokenParam = urlParams.get('token');
			const inviteParam = urlParams.get('invite');
			if (inviteParam) {
				this.inviteToken = inviteParam;
				this.mode = 'signup';
				fetch('/api/v1/invitations/' + inviteParam)
					.then(r => r.json())
					.then(data => {
						if (data.invitation) {
							this.inviteOrgName = data.invitation.org_name;
							if (data.invitation.email) this.signUpForm.email = data.invitation.email;
							if (data.invitation.role) this.signUpForm.role = data.invitation.role;
							this.successMessage = "You have been invited to join " + this.inviteOrgName + "! Complete your account below.";
						}
					})
					.catch(() => {});
			} else if (modeParam === 'reset' && tokenParam) {
				this.mode = 'reset';
				this.resetToken = tokenParam;
			} else if (modeParam === 'forgot') {
				this.mode = 'forgot';
			}
		},

		setMode(newMode) {
			this.mode = newMode;
			this.error = '';
			this.successMessage = '';
		},

		evaluatePassword(pwd) {
			if (!pwd) {
				this.criteria = { len: false, upper: false, lower: false, digit: false, special: false };
				this.passwordStrengthPct = 0;
				this.passwordStrengthText = '';
				return;
			}

			this.criteria.len = pwd.length >= 8;
			this.criteria.upper = /[A-Z]/.test(pwd);
			this.criteria.lower = /[a-z]/.test(pwd);
			this.criteria.digit = /[0-9]/.test(pwd);
			this.criteria.special = /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?\x60~]/.test(pwd);

			let score = 0;
			if (this.criteria.len) score += 20;
			if (this.criteria.upper) score += 20;
			if (this.criteria.lower) score += 20;
			if (this.criteria.digit) score += 20;
			if (this.criteria.special) score += 20;

			this.passwordStrengthPct = score;
			if (score < 60) {
				this.passwordStrengthText = 'Weak';
				this.passwordStrengthColor = 'text-red-600';
				this.passwordStrengthClass = 'bg-red-500';
			} else if (score < 100) {
				this.passwordStrengthText = 'Moderate';
				this.passwordStrengthColor = 'text-amber-600';
				this.passwordStrengthClass = 'bg-amber-500';
			} else {
				this.passwordStrengthText = 'Strong (Meets Policy)';
				this.passwordStrengthColor = 'text-emerald-700';
				this.passwordStrengthClass = 'bg-emerald-600';
			}
		},

		isPasswordValid(pwd) {
			return pwd.length >= 8 &&
				/[A-Z]/.test(pwd) &&
				/[a-z]/.test(pwd) &&
				/[0-9]/.test(pwd) &&
				/[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?\x60~]/.test(pwd);
		},

		async submitLogin() {
			this.loading = true;
			this.error = '';
			this.successMessage = '';
			try {
				const res = await fetch('/api/v1/auth/login', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						email: this.loginForm.email,
						password: this.loginForm.passphrase
					})
				});
				const data = await res.json();
				if (!res.ok) {
					this.error = data.message || 'Invalid email or password';
					return;
				}
				setTimeout(() => {
					window.location.href = '/dashboard';
				}, 400);
			} catch(err) {
				this.error = 'Network error. Please try again.';
			} finally {
				this.loading = false;
			}
		},

		async submitSignUp() {
			if (!this.isPasswordValid(this.signUpForm.passphrase)) {
				this.error = 'Password does not meet all security complexity requirements (8+ chars, uppercase, lowercase, number, and special symbol).';
				return;
			}
			if (this.signUpForm.passphrase !== this.signUpForm.confirmPassphrase) {
				this.error = 'Passwords do not match.';
				return;
			}
			this.loading = true;
			this.error = '';
			this.successMessage = '';
			try {
				const res = await fetch('/api/v1/auth/signup', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						name: this.signUpForm.name,
						email: this.signUpForm.email,
						password: this.signUpForm.passphrase,
						role: this.signUpForm.role,
						inviteToken: this.inviteToken || undefined
					})
				});
				const data = await res.json();
				if (!res.ok) {
					this.error = data.message || 'Failed to create account';
					return;
				}
				setTimeout(() => {
					window.location.href = '/dashboard';
				}, 400);
			} catch(err) {
				this.error = 'Network error. Please try again.';
			} finally {
				this.loading = false;
			}
		},

		async submitForgotPassword() {
			this.loading = true;
			this.error = '';
			this.successMessage = '';
			try {
				const res = await fetch('/api/v1/auth/forgot-password', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(this.forgotForm)
				});
				const data = await res.json();
				if (res.ok) {
					this.successMessage = data.message || 'Recovery instructions sent to your email.';
					this.forgotForm.email = '';
				} else {
					this.error = data.message || 'Failed to request reset.';
				}
			} catch(err) {
				this.error = 'Network error. Please try again.';
			} finally {
				this.loading = false;
			}
		},

		async submitResetPassword() {
			if (!this.isPasswordValid(this.resetForm.passphrase)) {
				this.error = 'Password does not meet all security complexity requirements (8+ chars, uppercase, lowercase, number, and special symbol).';
				return;
			}
			if (this.resetForm.passphrase !== this.resetForm.confirmPassphrase) {
				this.error = 'Passwords do not match.';
				return;
			}
			if (!this.resetToken) {
				this.error = 'Missing reset token in URL.';
				return;
			}
			this.loading = true;
			this.error = '';
			this.successMessage = '';
			try {
				const res = await fetch('/api/v1/auth/reset-password', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						token: this.resetToken,
						newPassword: this.resetForm.passphrase
					})
				});
				const data = await res.json();
				if (!res.ok) {
					this.error = data.message || 'Failed to reset password.';
					return;
				}
				this.successMessage = 'Password reset successfully! Redirecting to sign in...';
				setTimeout(() => {
					this.setMode('signin');
				}, 1500);
			} catch(err) {
				this.error = 'Network error. Please try again.';
			} finally {
				this.loading = false;
			}
		}
	}));

	// Experiment Modal Component
	Alpine.data('experimentModalComponent', () => ({
		isOpen: false,
		flagKey: '',
		metricName: 'conversion',
		report: { variant_stats: {}, winner_variant: '', control_variant: '' },
		getVariantStats() {
			return (this.report && this.report.variant_stats) || {};
		},
		isWinner(variant) {
			return Boolean(this.report && this.report.winner_variant === variant);
		},
		isControl(variant) {
			return Boolean(this.report && this.report.control_variant === variant);
		},
		getBadgeClass(variant) {
			if (this.isControl(variant)) return 'bg-slate-100 text-slate-700';
			if (this.isWinner(variant)) return 'bg-emerald-100 text-emerald-800';
			return 'bg-purple-100 text-purple-800';
		},
		getBadgeText(variant) {
			if (this.isControl(variant)) return 'CONTROL BASELINE';
			if (this.isWinner(variant)) return 'WINNER';
			return 'TREATMENT';
		},
		openModal(key) {
			this.flagKey = key;
			this.isOpen = true;
			this.fetchReport();
			this.$nextTick(() => {
				if (window.lucide) window.lucide.createIcons();
			});
		},
		closeModal() {
			this.isOpen = false;
		},
		async fetchReport() {
			if (!this.flagKey) return;
			try {
				const res = await fetch(`/api/v1/experiments/${this.flagKey}?metric=${encodeURIComponent(this.metricName)}`);
				if (res.ok) {
					this.report = await res.json();
					this.$nextTick(() => {
						if (window.lucide) window.lucide.createIcons();
					});
				}
			} catch (e) {
				// report fetch error handled gracefully
			}
		},
		async simulateConversion(variant) {
			try {
				await fetch('/api/v1/events', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						event: {
							flag_key: this.flagKey,
							variant: variant,
							metric_name: this.metricName,
							value: 1.0,
							environment: 'production',
						}
					})
				});
				this.fetchReport();
			} catch (e) {
				// simulation error handled gracefully
			}
		},
		// CSP-safe wrapper for Object.keys used in x-if binding
		hasComparisons() {
			return this.report && Object.keys(this.report.comparisons || {}).length > 0;
		},
		getWinningAction() {
			if (!this.report || !this.report.winner_variant || !this.report.comparisons) return '';
			const comp = this.report.comparisons[this.report.winner_variant];
			return (comp && comp.recommended_action) || '';
		}
	}));

	// Hygiene Refactor Diff Modal Component
	Alpine.data('hygieneModalComponent', () => ({
		isOpen: false,
		flagKey: '',
		flagName: '',
		flagStatus: 'LAUNCHED_100',
		flagReason: '',
		flagAction: 'archive',
		cleanupLang: 'go',
		copiedDiff: false,
		renderDiff() {
			if (this.$refs.diffBox) {
				this.$refs.diffBox.innerHTML = this.getDiffHtml();
			}
		},
		openModal(data) {
			this.flagKey = data.key || '';
			this.flagName = data.name || '';
			this.flagStatus = data.status || 'READY_FOR_CLEANUP';
			this.flagReason = data.reason || '';
			this.flagAction = data.action || '';
			this.isOpen = true;
			this.$nextTick(() => {
				if (window.lucide) window.lucide.createIcons();
			});
		},
		closeModal() {
			this.isOpen = false;
		},
		getDiffHtml() {
			const escapeHtml = (str) => {
				if (!str) return '';
				return String(str)
					.replace(/&/g, '&amp;')
					.replace(/</g, '&lt;')
					.replace(/>/g, '&gt;')
					.replace(/"/g, '&quot;')
					.replace(/'/g, '&#39;');
			};
			const k = escapeHtml(this.flagKey || 'my-feature-flag');
			if (this.cleanupLang === 'go') {
				if (this.flagStatus === 'DEAD_FLAG') {
					return '<span class="text-red-400 font-bold">- if client.IsEnabled(ctx, "' + k + '", userCtx) {</span>\n' +
						'<span class="text-red-400 font-bold">-     doFeatureWork()</span>\n' +
						'<span class="text-red-400 font-bold">- } else {</span>\n' +
						'<span class="text-emerald-400 font-bold">+ // Feature abandoned / dead code removed:</span>\n' +
						'<span class="text-emerald-400 font-bold">+ doFallbackWork()</span>\n' +
						'<span class="text-red-400 font-bold">-     doFallbackWork()</span>\n' +
						'<span class="text-red-400 font-bold">- }</span>';
				}
				return '<span class="text-red-400 font-bold">- if client.IsEnabled(ctx, "' + k + '", userCtx) {</span>\n' +
					'<span class="text-red-400 font-bold">-     renderNewExperience()</span>\n' +
					'<span class="text-red-400 font-bold">- } else {</span>\n' +
					'<span class="text-red-400 font-bold">-     renderLegacyExperience()</span>\n' +
					'<span class="text-red-400 font-bold">- }</span>\n' +
					'<span class="text-emerald-400 font-bold">+ // 100% launched: permanent in-line code</span>\n' +
					'<span class="text-emerald-400 font-bold">+ renderNewExperience()</span>';
			} else if (this.cleanupLang === 'ts') {
				if (this.flagStatus === 'DEAD_FLAG') {
					return '<span class="text-red-400 font-bold">- if (await client.evaluate("' + k + '", user)) {</span>\n' +
						'<span class="text-red-400 font-bold">-   executeFeature();</span>\n' +
						'<span class="text-red-400 font-bold">- } else {</span>\n' +
						'<span class="text-emerald-400 font-bold">+ executeFallback();</span>\n' +
						'<span class="text-red-400 font-bold">-   executeFallback();</span>\n' +
						'<span class="text-red-400 font-bold">- }</span>';
				}
				return '<span class="text-red-400 font-bold">- if (await client.evaluate("' + k + '", user)) {</span>\n' +
					'<span class="text-red-400 font-bold">-   renderNewUI();</span>\n' +
					'<span class="text-red-400 font-bold">- } else {</span>\n' +
					'<span class="text-red-400 font-bold">-   renderLegacyUI();</span>\n' +
					'<span class="text-red-400 font-bold">- }</span>\n' +
					'<span class="text-emerald-400 font-bold">+ // Permanent mainline code:</span>\n' +
					'<span class="text-emerald-400 font-bold">+ renderNewUI();</span>';
			} else {
				if (this.flagStatus === 'DEAD_FLAG') {
					return '<span class="text-red-400 font-bold">- if client.is_enabled("' + k + '", user):</span>\n' +
						'<span class="text-red-400 font-bold">-     run_experimental_feature()</span>\n' +
						'<span class="text-red-400 font-bold">- else:</span>\n' +
						'<span class="text-emerald-400 font-bold">+ run_default_feature()</span>\n' +
						'<span class="text-red-400 font-bold">-     run_default_feature()</span>';
				}
				return '<span class="text-red-400 font-bold">- if client.is_enabled("' + k + '", user):</span>\n' +
					'<span class="text-red-400 font-bold">-     run_new_experience()</span>\n' +
					'<span class="text-red-400 font-bold">- else:</span>\n' +
					'<span class="text-red-400 font-bold">-     run_legacy_experience()</span>\n' +
					'<span class="text-emerald-400 font-bold">+ # 100% launched: permanent mainline code</span>\n' +
					'<span class="text-emerald-400 font-bold">+ run_new_experience()</span>';
			}
		},
		getCleanCode() {
			if (this.flagStatus === 'DEAD_FLAG') {
				if (this.cleanupLang === 'go') return 'doFallbackWork()';
				if (this.cleanupLang === 'ts') return 'executeFallback();';
				return 'run_default_feature()';
			}
			if (this.cleanupLang === 'go') return 'renderNewExperience()';
			if (this.cleanupLang === 'ts') return 'renderNewUI();';
			return 'run_new_experience()';
		},
		copyDiffSnippet() {
			navigator.clipboard.writeText(this.getCleanCode());
			this.copiedDiff = true;
			setTimeout(() => this.copiedDiff = false, 2000);
		},
		async deleteFlag(key) {
			if (!confirm(`Permanently delete feature flag '${key}' from database?`)) return;
			try {
				const res = await fetch('/api/v1/flags/' + key, { method: 'DELETE' });
				if (res.ok) {
					this.closeModal();
					window.location.reload();
				} else {
					alert('Failed to delete flag: ' + (await res.text()));
				}
			} catch (e) {
				alert('Network error: ' + e.message);
			}
		}
	}));

	// Profile Settings Component
	function profileSettingsComponent() {
		return {
			tab: 'general',
			savingProfile: false,
			savingPassword: false,
			profileMessage: '',
			passwordError: '',
			passwordSuccess: '',
			showCurrentPassword: false,
			showNewPassword: false,
			profile: {
				id: '',
				name: '',
				email: '',
				role: '',
				avatarUrl: ''
			},
			form: {
				name: '',
				avatarUrl: ''
			},
			passwordForm: {
				currentPassword: '',
				newPassword: '',
				confirmPassword: ''
			},
			get passRules() {
				const p = this.passwordForm.newPassword || '';
				return {
					length: p.length >= 8,
					upper: /[A-Z]/.test(p),
					lower: /[a-z]/.test(p),
					digit: /[0-9]/.test(p),
					special: /[!@#$%^&*()_+\-=\[\]{}|;:,.<>?~]/.test(p)
				};
			},
			isPasswordValid() {
				const r = this.passRules;
				return r.length && r.upper && r.lower && r.digit && r.special &&
					this.passwordForm.currentPassword.length > 0 &&
					this.passwordForm.newPassword === this.passwordForm.confirmPassword;
			},
			init() {
				this.fetchProfile();
			},
			getInitials(name) {
				if (!name) return 'U';
				const parts = name.trim().split(/\s+/);
				if (parts.length >= 2) {
					return (parts[0][0] + parts[1][0]).toUpperCase();
				}
				return name.substring(0, 2).toUpperCase();
			},
			async fetchProfile() {
				try {
					const res = await fetch('/api/v1/auth/me');
					if (res.ok) {
						const data = await res.json();
						this.profile = data;
						this.form.name = data.name || '';
						this.form.avatarUrl = data.avatarUrl || '';
					}
				} catch(e) {}
			},
			resetForm() {
				this.form.name = this.profile.name || '';
				this.form.avatarUrl = this.profile.avatarUrl || '';
				this.profileMessage = '';
			},
			async saveProfile() {
				if (!this.form.name.trim()) return;
				this.savingProfile = true;
				this.profileMessage = '';
				try {
					const res = await fetch('/api/v1/auth/profile', {
						method: 'PATCH',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							name: this.form.name.trim(),
							avatarUrl: this.form.avatarUrl.trim()
						})
					});
					const data = await res.json();
					if (!res.ok) {
						throw new Error(data.message || 'Failed to update profile');
					}
					this.profile.name = this.form.name.trim();
					this.profile.avatarUrl = this.form.avatarUrl.trim();
					this.showToast('Profile updated successfully!', 'info');
					this.profileMessage = 'Profile updated successfully!';
				} catch (err) {
					this.profileMessage = err.message;
					this.showToast(err.message, 'error');
				} finally {
					this.savingProfile = false;
				}
			},
			async changePassword() {
				if (!this.isPasswordValid()) return;
				this.savingPassword = true;
				this.passwordError = '';
				this.passwordSuccess = '';
				try {
					const res = await fetch('/api/v1/auth/change-password', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							currentPassword: this.passwordForm.currentPassword,
							newPassword: this.passwordForm.newPassword
						})
					});
					const data = await res.json();
					if (!res.ok) {
						throw new Error(data.message || 'Failed to update password');
					}
					this.passwordSuccess = 'Password has been successfully updated!';
					this.passwordForm.currentPassword = '';
					this.passwordForm.newPassword = '';
					this.passwordForm.confirmPassword = '';
					this.showToast('Password updated successfully!', 'info');
				} catch (err) {
					this.passwordError = err.message;
					this.showToast(err.message, 'error');
				} finally {
					this.savingPassword = false;
				}
			},
			copyToClipboard(text, msg = 'Copied to clipboard') {
				if (!text) return;
				navigator.clipboard.writeText(text).then(() => {
					this.showToast(msg, 'info');
				});
			}
		};
	}
	window.profileSettingsComponent = profileSettingsComponent;
	Alpine.data('profileSettingsComponent', profileSettingsComponent);
});

// ==========================================
// 3. UI Enhancements: Custom Cursor, Three.js, Lucide
// ==========================================

document.addEventListener('DOMContentLoaded', () => {
	// Initialize Lucide Icons
	if (window.lucide) window.lucide.createIcons();

	// Initialize Custom Cursor (if elements present)
	const dot = document.getElementById('custom-cursor-dot');
	const ring = document.getElementById('custom-cursor-ring');
	if (dot && ring) {
		let mouseX = -100, mouseY = -100;
		let ringX = -100, ringY = -100;

		window.addEventListener('mousemove', (e) => {
			mouseX = e.clientX;
			mouseY = e.clientY;
			dot.style.transform = `translate3d(${mouseX}px, ${mouseY}px, 0) translate(-50%, -50%)`;
		});

		const renderRing = () => {
			ringX += (mouseX - ringX) * 0.15;
			ringY += (mouseY - ringY) * 0.15;
			ring.style.transform = `translate3d(${ringX}px, ${ringY}px, 0) translate(-50%, -50%)`;
			requestAnimationFrame(renderRing);
		};
		renderRing();

		document.querySelectorAll('button, a, input, select, textarea, .clickable, [role="button"]').forEach(el => {
			el.addEventListener('mouseenter', () => {
				ring.classList.add('scale-150', 'border-[#00F5A0]', 'bg-[#00F5A0]/10');
			});
			el.addEventListener('mouseleave', () => {
				ring.classList.remove('scale-150', 'border-[#00F5A0]', 'bg-[#00F5A0]/10');
			});
		});
	}

	// Initialize Three.js particle canvas (if elements and library present)
	const canvas = document.getElementById('three-bg-canvas');
	if (canvas && window.THREE) {
		const scene = new THREE.Scene();
		const camera = new THREE.PerspectiveCamera(60, window.innerWidth / window.innerHeight, 0.1, 1000);
		camera.position.z = 24;

		const renderer = new THREE.WebGLRenderer({ canvas: canvas, alpha: true, antialias: true });
		renderer.setSize(window.innerWidth, window.innerHeight);
		renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

		const particlesCount = 900;
		const positions = new Float32Array(particlesCount * 3);
		const colors = new Float32Array(particlesCount * 3);

		const color1 = new THREE.Color('#00F5A0');
		const color2 = new THREE.Color('#00D2FF');
		const color3 = new THREE.Color('#8B5CF6');

		for (let i = 0; i < particlesCount; i++) {
			const i3 = i * 3;
			const radius = 10 + Math.random() * 8;
			const theta = Math.random() * Math.PI * 2;
			const phi = Math.acos((Math.random() * 2) - 1);

			positions[i3] = radius * Math.sin(phi) * Math.cos(theta);
			positions[i3 + 1] = radius * Math.sin(phi) * Math.sin(theta);
			positions[i3 + 2] = radius * Math.cos(phi);

			const rand = Math.random();
			let mixedColor;
			if (rand < 0.45) {
				mixedColor = color1.clone().lerp(color2, Math.random());
			} else if (rand < 0.8) {
				mixedColor = color2.clone().lerp(color3, Math.random());
			} else {
				mixedColor = color3.clone().lerp(color1, Math.random());
			}
			colors[i3] = mixedColor.r;
			colors[i3 + 1] = mixedColor.g;
			colors[i3 + 2] = mixedColor.b;
		}

		const geometry = new THREE.BufferGeometry();
		geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
		geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3));

		const material = new THREE.PointsMaterial({
			size: 0.16,
			vertexColors: true,
			transparent: true,
			opacity: 0.8,
			blending: THREE.AdditiveBlending
		});

		const particlesMesh = new THREE.Points(geometry, material);
		scene.add(particlesMesh);

		let mouseTargetX = 0;
		let mouseTargetY = 0;
		window.addEventListener('mousemove', (event) => {
			mouseTargetX = (event.clientX / window.innerWidth - 0.5) * 2;
			mouseTargetY = -(event.clientY / window.innerHeight - 0.5) * 2;
		});

		let clock = new THREE.Clock();
		const animate = () => {
			requestAnimationFrame(animate);
			const elapsedTime = clock.getElapsedTime();
			particlesMesh.rotation.y = elapsedTime * 0.04;
			particlesMesh.rotation.x = elapsedTime * 0.02;

			particlesMesh.rotation.y += (mouseTargetX * 0.5 - particlesMesh.rotation.y) * 0.03;
			particlesMesh.rotation.x += (-mouseTargetY * 0.5 - particlesMesh.rotation.x) * 0.03;

			renderer.render(scene, camera);
		};
		animate();

		window.addEventListener('resize', () => {
			camera.aspect = window.innerWidth / window.innerHeight;
			camera.updateProjectionMatrix();
			renderer.setSize(window.innerWidth, window.innerHeight);
			renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
		});
	}
});

document.addEventListener('alpine:initialized', () => {
	if (window.lucide) window.lucide.createIcons();
});
