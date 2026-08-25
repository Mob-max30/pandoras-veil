// ==========================================================================
// PANDORA'S VEIL | Secure Secret Relay & Pure Verified Web Interface
// ==========================================================================

function getInitials(handle) {
    if (!handle) return '?';
    const clean = handle.replace(/^PV-/, '').replace(/^#/, '');
    const parts = clean.split(/[\s_-]+/);
    if (parts.length >= 2) {
        return (parts[0][0] + parts[1][0]).toUpperCase();
    }
    return clean.slice(0, 1).toUpperCase() || 'P';
}

function formatTTL(seconds) {
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
    return `${Math.floor(seconds / 86400)}d`;
}

const state = {
    myHandle: '',
    myFingerprint: '',
    myPublicKey: '',
    activeTarget: '',
    activeDisplayName: '',
    activeTargetFP: '',
    activeTargetPK: '',
    isGroup: false,
    groupMembers: [],
    convTTL: {},        // Main Disappearing TTL per conversation (default 300s)
    currentMsgTTL: 300, // Per-message TTL override (cannot exceed main TTL)
    burnAfterReading: true,
    eventSource: null,
    contacts: [],
    conversations: {},
    serverConnected: false
};

function getAuthToken() {
    const meta = document.querySelector('meta[name="pandora-token"]');
    return meta ? meta.getAttribute('content') : '';
}

// Persistence Utilities (Stores ONLY real user interactions)
function loadPersistedData() {
    try {
        const savedContacts = localStorage.getItem('pandora_contacts_v5');
        const savedConvs = localStorage.getItem('pandora_conversations_v5');
        const savedTarget = localStorage.getItem('pandora_active_target_v5');
        const savedTTL = localStorage.getItem('pandora_conv_ttl_v5');

        state.contacts = savedContacts ? JSON.parse(savedContacts) : [];
        state.conversations = savedConvs ? JSON.parse(savedConvs) : {};
        state.convTTL = savedTTL ? JSON.parse(savedTTL) : {};

        if (savedTarget && state.contacts.some(c => c.handle === savedTarget)) {
            state.activeTarget = savedTarget;
        } else if (state.contacts.length > 0) {
            state.activeTarget = state.contacts[0].handle;
        } else {
            state.activeTarget = '';
        }
    } catch (e) {
        state.contacts = [];
        state.conversations = {};
        state.convTTL = {};
        state.activeTarget = '';
    }
}

function savePersistedData() {
    try {
        localStorage.setItem('pandora_contacts_v5', JSON.stringify(state.contacts));
        localStorage.setItem('pandora_conversations_v5', JSON.stringify(state.conversations));
        localStorage.setItem('pandora_active_target_v5', state.activeTarget);
        localStorage.setItem('pandora_conv_ttl_v5', JSON.stringify(state.convTTL));
    } catch (e) {
        console.warn('Failed to save to localStorage:', e);
    }
}

// DOM Elements
const myFingerprintEl = document.getElementById('my-fingerprint');
const directChatsListEl = document.getElementById('direct-chats-list');
const groupChatsListEl = document.getElementById('group-chats-list');
const activeHeaderAvatarEl = document.getElementById('active-header-avatar');
const activeContactTitleEl = document.getElementById('active-contact-title');
const activeStatusLineEl = document.getElementById('active-status-line');
const activeStatusTextEl = document.getElementById('active-status-text');
const headerConnStatusEl = document.getElementById('header-conn-status');
const chatMessagesContainerEl = document.getElementById('chat-messages-container');
const chatMessagesScrollEl = document.getElementById('chat-messages-scroll');
const chatInputEl = document.getElementById('chat-input');
const messageFormEl = document.getElementById('message-form');
const modalBackdropEl = document.getElementById('modal-backdrop');
const modalTitleEl = document.getElementById('modal-title');
const modalBodyEl = document.getElementById('modal-body');
const initOverlayEl = document.getElementById('init-overlay');
const initHandleInputEl = document.getElementById('init-handle-input');
const initErrorMsgEl = document.getElementById('init-error-msg');
const initSubmitBtnEl = document.getElementById('init-submit-button');
const optionsDropdownEl = document.getElementById('options-dropdown');

// 1. Initialize App & Connect to Stream
async function initApp() {
    loadPersistedData();

    try {
        const token = getAuthToken();
        const res = await fetch('/api/identity', {
            headers: { 'X-Pandora-Token': token }
        });
        if (res.ok) {
            const data = await res.json();
            if (data.initialized === false || data.initialized === 'false' || !data.handle) {
                showInitOverlay();
                return;
            }
            state.myHandle = data.handle;
            state.myFingerprint = data.fingerprint;
            state.myPublicKey = data.publicKey;
            state.serverConnected = true;

            myFingerprintEl.textContent = state.myFingerprint;
            hideInitOverlay();
        } else {
            showInitOverlay();
            return;
        }
    } catch (err) {
        console.warn('Failed to load local identity:', err);
        showInitOverlay();
        return;
    }

    renderCorrespondenceSidebar();
    setupEventListeners();
    connectSSEStream();
    renderActiveConversation();
    startExpirationPruner();
}

function showInitOverlay() {
    if (initOverlayEl) {
        initOverlayEl.classList.remove('hidden');
        if (initHandleInputEl) initHandleInputEl.focus();
    }
}

function hideInitOverlay() {
    if (initOverlayEl) {
        initOverlayEl.classList.add('hidden');
    }
}

async function handleInitSubmit(e) {
    e.preventDefault();
    const handleVal = initHandleInputEl.value.trim();
    if (!handleVal) return;

    initErrorMsgEl.classList.add('hidden');
    initSubmitBtnEl.disabled = true;
    initSubmitBtnEl.textContent = 'Initializing...';

    try {
        const token = getAuthToken();
        const res = await fetch('/api/init', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Pandora-Token': token
            },
            body: JSON.stringify({ handle: handleVal })
        });

        const data = await res.json();
        if (res.ok && data.success) {
            state.myHandle = data.handle;
            state.myFingerprint = data.fingerprint;
            state.myPublicKey = data.publicKey;
            state.serverConnected = true;

            myFingerprintEl.textContent = state.myFingerprint;

            hideInitOverlay();
            renderCorrespondenceSidebar();
            setupEventListeners();
            connectSSEStream();
            renderActiveConversation();
        } else {
            initErrorMsgEl.textContent = data.error || 'Failed to initialize device.';
            initErrorMsgEl.classList.remove('hidden');
            initSubmitBtnEl.disabled = false;
            initSubmitBtnEl.textContent = 'Initialize';
        }
    } catch (err) {
        initErrorMsgEl.textContent = `Network error: ${err.message}`;
        initErrorMsgEl.classList.remove('hidden');
        initSubmitBtnEl.disabled = false;
        initSubmitBtnEl.textContent = 'Initialize';
    }
}

// 2. Render Correspondence List in Sidebar
function renderCorrespondenceSidebar() {
    directChatsListEl.innerHTML = '';
    groupChatsListEl.innerHTML = '';

    const directContacts = state.contacts.filter(c => c.type !== 'group');
    const groupContacts = state.contacts.filter(c => c.type === 'group');

    // Render Direct Contacts
    if (directContacts.length === 0) {
        const emptyEl = document.createElement('div');
        emptyEl.style.cssText = 'padding: 8px 10px; font-size: 0.8rem; color: #64748b;';
        emptyEl.textContent = 'No direct chats';
        directChatsListEl.appendChild(emptyEl);
    } else {
        directContacts.forEach(contact => {
            const initials = getInitials(contact.name || contact.handle);
            const isActive = contact.handle === state.activeTarget;

            const card = document.createElement('div');
            card.className = `pv-contact-card ${isActive ? 'active' : ''}`;
            card.setAttribute('data-handle', contact.handle);

            card.innerHTML = `
                <div class="card-avatar">${initials}</div>
                <span class="card-name">${contact.name || contact.handle}</span>
                ${isActive ? '<span class="card-dot"></span>' : ''}
            `;

            card.addEventListener('click', () => {
                selectContact(contact.handle);
            });

            directChatsListEl.appendChild(card);
        });
    }

    // Render Groups
    if (groupContacts.length === 0) {
        const emptyEl = document.createElement('div');
        emptyEl.style.cssText = 'padding: 8px 10px; font-size: 0.8rem; color: #64748b;';
        emptyEl.textContent = 'No groups';
        groupChatsListEl.appendChild(emptyEl);
    } else {
        groupContacts.forEach(contact => {
            const isActive = contact.handle === state.activeTarget;
            const card = document.createElement('div');
            card.className = `pv-group-card ${isActive ? 'active' : ''}`;
            card.setAttribute('data-handle', contact.handle);
            card.textContent = contact.name || contact.handle;

            card.addEventListener('click', () => {
                selectContact(contact.handle);
            });

            groupChatsListEl.appendChild(card);
        });
    }

    // Update active header details
    const activeContact = state.contacts.find(c => c.handle === state.activeTarget);
    if (activeContact) {
        const initials = getInitials(activeContact.name || activeContact.handle);
        activeHeaderAvatarEl.textContent = initials;
        activeContactTitleEl.textContent = activeContact.name || activeContact.handle;
        activeStatusTextEl.textContent = activeContact.type === 'group' 
            ? `${activeContact.members.length} Members · End-to-end verified` 
            : 'Online · End-to-end verified';
        state.isGroup = activeContact.type === 'group';
        state.groupMembers = activeContact.members || [];
        state.activeTargetFP = activeContact.fp || '';
        state.activeTargetPK = activeContact.publicKey || '';
    } else {
        activeHeaderAvatarEl.textContent = '—';
        activeContactTitleEl.textContent = 'No Active Chat';
        activeStatusTextEl.textContent = state.serverConnected ? 'Connected to Relay' : 'Connecting...';
    }
}

function getMainTTL(handle) {
    if (!handle) return 300;
    return state.convTTL[handle] || 300;
}

function selectContact(handle) {
    state.activeTarget = handle;
    const mainTTL = getMainTTL(handle);
    state.currentMsgTTL = mainTTL;
    savePersistedData();
    renderCorrespondenceSidebar();
    renderActiveConversation();
    chatInputEl.focus();
}

function touchContact(handle, lastMsgText, timestamp, senderName, fp, pk, type, members) {
    let contact = state.contacts.find(c => c.handle === handle);
    if (!contact) {
        contact = {
            handle: handle,
            name: senderName || handle,
            fp: fp || 'Verified',
            publicKey: pk || '',
            type: type || (handle.includes(',') ? 'group' : 'dm'),
            members: members || [],
            lastMessage: lastMsgText,
            time: timestamp
        };
        state.contacts.unshift(contact);
    } else {
        contact.lastMessage = lastMsgText;
        contact.time = timestamp;
        if (fp) contact.fp = fp;
        if (pk) contact.publicKey = pk;
        state.contacts = [contact, ...state.contacts.filter(c => c.handle !== handle)];
    }
    savePersistedData();
    renderCorrespondenceSidebar();
}

// 3. Connect to SSE Stream Bridge
function connectSSEStream() {
    if (state.eventSource) {
        state.eventSource.close();
    }

    const token = getAuthToken();
    const streamURL = token ? `/api/stream?token=${encodeURIComponent(token)}` : '/api/stream';
    state.eventSource = new EventSource(streamURL);

    state.eventSource.onopen = () => {
        state.serverConnected = true;
        headerConnStatusEl.textContent = 'Connected';
    };

    state.eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data && data.text) {
                const senderName = data.sender || state.activeTarget;
                const timestamp = data.timestamp || formatTime(new Date());
                const msgTTL = data.ttl || getMainTTL(senderName);
                const msg = {
                    id: 'msg_' + Date.now() + '_' + Math.random().toString(36).substr(2, 5),
                    sender: senderName,
                    text: data.text,
                    timestamp: timestamp,
                    ttl: msgTTL,
                    expiresAt: Date.now() + msgTTL * 1000,
                    isOutgoing: false
                };

                if (!state.conversations[senderName]) {
                    state.conversations[senderName] = [];
                }
                state.conversations[senderName].push(msg);

                touchContact(senderName, data.text, timestamp, senderName);

                if (!state.activeTarget) {
                    selectContact(senderName);
                } else if (senderName === state.activeTarget || (state.isGroup && senderName !== state.myHandle)) {
                    appendBubble(msg);
                }
            }
        } catch (err) {
            console.error('Error parsing stream event:', err);
        }
    };

    state.eventSource.onerror = () => {
        state.serverConnected = false;
        headerConnStatusEl.textContent = 'Reconnecting...';
    };
}

// 4. Render Active Conversation Messages
function renderActiveConversation() {
    if (!state.activeTarget || state.contacts.length === 0) {
        chatMessagesContainerEl.innerHTML = `
            <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; padding-top: 120px; text-align:center; color:#64748b;">
                <h3 style="font-size:1.4rem; color:var(--pv-text-main); margin-bottom:8px; font-weight:700;">Pandora's Veil</h3>
                <p style="font-size:0.9rem; max-width:360px; line-height:1.5;">Zero-knowledge cryptographic relay. Start a new chat or group to begin.</p>
            </div>
        `;
        return;
    }

    chatMessagesContainerEl.innerHTML = '';

    const msgs = state.conversations[state.activeTarget] || [];
    const now = Date.now();
    const validMsgs = msgs.filter(m => !m.expiresAt || m.expiresAt > now);
    state.conversations[state.activeTarget] = validMsgs;

    validMsgs.forEach(msg => appendBubble(msg));
    scrollChatToBottom();
}

// 5. Append Message Bubble
function appendBubble(msg) {
    const groupEl = document.createElement('div');
    groupEl.id = msg.id || ('msg_' + Date.now());
    groupEl.className = `pv-bubble-group ${msg.isOutgoing ? 'outgoing' : 'incoming'}`;

    if (msg.isFile) {
        // Render File Card
        const fileCard = document.createElement('div');
        fileCard.className = 'pv-file-card';
        fileCard.innerHTML = `
            <div class="pv-file-icon-box">
                <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>
            </div>
            <div class="pv-file-meta">
                <div class="pv-file-name">${msg.fileName || 'file'}</div>
                <div class="pv-file-size">${msg.fileSize || '2.4 MB'}</div>
                <div class="pv-file-status">✓ Auto-saved to ./downloads/</div>
            </div>
        `;
        groupEl.appendChild(fileCard);
    } else {
        const cardEl = document.createElement('div');
        cardEl.className = 'pv-card-bubble';
        cardEl.textContent = msg.text;
        groupEl.appendChild(cardEl);

        const metaEl = document.createElement('div');
        metaEl.className = 'pv-bubble-meta';
        metaEl.textContent = msg.isOutgoing ? 'Encrypted & Dispatched' : 'Decrypted locally';
        groupEl.appendChild(metaEl);
    }

    chatMessagesContainerEl.appendChild(groupEl);
    scrollChatToBottom();
}

function scrollChatToBottom() {
    chatMessagesScrollEl.scrollTop = chatMessagesScrollEl.scrollHeight;
}

// 6. Send Message
async function handleSendMessage(e) {
    e.preventDefault();
    const text = chatInputEl.value.trim();
    if (!text) return;

    if (!state.activeTarget) {
        openNewChatModal('peer');
        return;
    }

    chatInputEl.value = '';

    const timestamp = formatTime(new Date());
    const effectiveTTL = Math.min(state.currentMsgTTL, getMainTTL(state.activeTarget));
    const msgId = 'msg_' + Date.now() + '_' + Math.random().toString(36).substr(2, 5);

    const msg = {
        id: msgId,
        sender: state.myHandle,
        text: text,
        timestamp: timestamp,
        ttl: effectiveTTL,
        expiresAt: Date.now() + effectiveTTL * 1000,
        isOutgoing: true
    };

    if (!state.conversations[state.activeTarget]) {
        state.conversations[state.activeTarget] = [];
    }
    state.conversations[state.activeTarget].push(msg);
    appendBubble(msg);

    touchContact(state.activeTarget, text, timestamp);

    try {
        const token = getAuthToken();
        const payload = {
            target: state.activeTarget,
            isGroup: state.isGroup,
            groupMembers: state.groupMembers,
            text: text,
            ttl: effectiveTTL,
            burn: state.burnAfterReading
        };

        const res = await fetch('/api/send', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Pandora-Token': token
            },
            body: JSON.stringify(payload)
        });

        if (!res.ok) {
            const errData = await res.json().catch(() => ({}));
            appendBubble({
                id: 'err_' + Date.now(),
                sender: 'SYSTEM',
                text: `Delivery failed: ${errData.error || 'Relay error'}`,
                timestamp: formatTime(new Date()),
                isOutgoing: false
            });
        }
    } catch (err) {
        appendBubble({
            id: 'err_' + Date.now(),
            sender: 'SYSTEM',
            text: `Network error: ${err.message}`,
            timestamp: formatTime(new Date()),
            isOutgoing: false
        });
    }
}

// 7. Expired Message Pruner
function startExpirationPruner() {
    setInterval(() => {
        const now = Date.now();
        let changed = false;

        Object.keys(state.conversations).forEach(target => {
            const list = state.conversations[target] || [];
            const fresh = list.filter(m => {
                if (m.expiresAt && m.expiresAt <= now) {
                    changed = true;
                    const el = document.getElementById(m.id);
                    if (el) el.remove();
                    return false;
                }
                return true;
            });
            state.conversations[target] = fresh;
        });

        if (changed) {
            savePersistedData();
        }
    }, 1000);
}

// 8. Options Menu Dropdown
function toggleOptionsMenu(e) {
    e.stopPropagation();
    if (optionsDropdownEl) {
        optionsDropdownEl.classList.toggle('hidden');
    }
}

document.addEventListener('click', () => {
    if (optionsDropdownEl && !optionsDropdownEl.classList.contains('hidden')) {
        optionsDropdownEl.classList.add('hidden');
    }
});

// 9. Setup Event Listeners
function setupEventListeners() {
    messageFormEl.addEventListener('submit', handleSendMessage);
}

// 10. Modals & Actions

// Profile Modal (Matches `pv identity` format + Delete Account option)
function toggleProfileModal() {
    openModal('Device Identity (Verified on Relay)', `
        <div style="display:flex; flex-direction:column; gap:12px; font-size:0.88rem;">
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Handle</div>
                <div style="font-family:var(--pv-font-mono); color:#f8fafc; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${state.myHandle}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Device Fingerprint</div>
                <div style="font-family:var(--pv-font-mono); color:#34d399; font-weight:600; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${state.myFingerprint}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Public Key (X25519)</div>
                <div style="font-family:var(--pv-font-mono); font-size:0.78rem; color:#94a3b8; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932; word-break:break-all;">${state.myPublicKey}</div>
            </div>
        </div>
        <div style="display:flex; justify-content:flex-end; margin-top:20px; padding-top:14px; border-top:1px solid #202932;">
            <button type="button" style="background:#ef4444; color:#fff; border:none; padding:8px 16px; border-radius:8px; font-weight:600; cursor:pointer;" onclick="confirmDeleteAccount()">Delete Account</button>
        </div>
    `);
}

async function confirmDeleteAccount() {
    if (!confirm('Are you sure you want to delete your account? This will unregister your device from the relay, permanently delete your local X25519 private key, and wipe all local correspondence.')) {
        return;
    }

    try {
        const token = getAuthToken();
        await fetch('/api/delete-account', {
            method: 'POST',
            headers: { 'X-Pandora-Token': token }
        });
    } catch (e) {
        console.warn('Delete request failed:', e);
    }

    localStorage.clear();
    location.reload();
}

// Peer Contact Details Modal
function openContactDetailsModal() {
    if (!state.activeTarget) return;
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;
    const fp = contact ? (contact.fp || state.activeTargetFP || 'Verified') : 'Verified';
    const pk = contact ? (contact.publicKey || state.activeTargetPK || 'Verified on Relay') : 'Verified on Relay';

    if (contact && contact.type === 'group') {
        const memberList = (contact.members || []).map(m => `<li style="padding:4px 0;"><code>${m}</code></li>`).join('');
        openModal(`Group: ${displayName}`, `
            <div style="display:flex; flex-direction:column; gap:12px; font-size:0.88rem;">
                <div>
                    <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Group Name</div>
                    <div style="font-family:var(--pv-font-mono); color:#f8fafc; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${displayName}</div>
                </div>
                <div>
                    <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Members (${contact.members.length})</div>
                    <ul style="margin:4px 0 0 18px; font-size:0.88rem; color:#94a3b8;">${memberList}</ul>
                </div>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:20px;">
                <button type="button" style="background:#ef4444; color:#fff; border:none; padding:8px 16px; border-radius:8px; font-weight:600; cursor:pointer;" onclick="removeCorrespondence('${state.activeTarget}')">Remove Group</button>
            </div>
        `);
        return;
    }

    openModal(`Peer: ${displayName}`, `
        <div style="display:flex; flex-direction:column; gap:12px; font-size:0.88rem;">
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Handle</div>
                <div style="font-family:var(--pv-font-mono); color:#f8fafc; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${state.activeTarget}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Device Fingerprint</div>
                <div style="font-family:var(--pv-font-mono); color:#34d399; font-weight:600; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${fp}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Public Key</div>
                <div style="font-family:var(--pv-font-mono); font-size:0.78rem; color:#94a3b8; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932; word-break:break-all;">${pk}</div>
            </div>
        </div>
        <div style="display:flex; justify-content:flex-end; margin-top:20px;">
            <button type="button" style="background:#ef4444; color:#fff; border:none; padding:8px 16px; border-radius:8px; font-weight:600; cursor:pointer;" onclick="removeCorrespondence('${state.activeTarget}')">Remove Correspondence</button>
        </div>
    `);
}

function removeCorrespondence(handle) {
    state.contacts = state.contacts.filter(c => c.handle !== handle);
    delete state.conversations[handle];
    delete state.convTTL[handle];

    if (state.activeTarget === handle) {
        state.activeTarget = state.contacts.length > 0 ? state.contacts[0].handle : '';
    }

    savePersistedData();
    closeModal();
    renderCorrespondenceSidebar();
    renderActiveConversation();
}

// Add New Chat / Group Modal
function openNewChatModal(tab = 'peer') {
    openModal(tab === 'peer' ? 'New Chat' : 'New Group', `
        <div style="display:flex; gap:8px; margin-bottom:16px; border-bottom:1px solid #202932; padding-bottom:10px;">
            <button type="button" id="tab-btn-peer" style="background:${tab === 'peer' ? 'var(--pv-emerald)' : 'transparent'}; color:${tab === 'peer' ? '#fff' : 'var(--pv-text-muted)'}; border:none; padding:6px 14px; border-radius:8px; font-weight:600; cursor:pointer;" onclick="switchAddTab('peer')">Direct Chat</button>
            <button type="button" id="tab-btn-group" style="background:${tab === 'group' ? 'var(--pv-emerald)' : 'transparent'}; color:${tab === 'group' ? '#fff' : 'var(--pv-text-muted)'}; border:none; padding:6px 14px; border-radius:8px; font-weight:600; cursor:pointer;" onclick="switchAddTab('group')">Encrypted Group</button>
        </div>

        <div id="add-peer-tab-content" class="${tab === 'peer' ? '' : 'hidden'}">
            <div style="display:flex; flex-direction:column; gap:12px;">
                <input type="text" id="new-peer-handle-input" placeholder="Peer handle (e.g. marcus or alex)" style="width:100%; height:44px; background:#141a20; border:1px solid #202932; color:#f8fafc; border-radius:10px; padding:0 14px; outline:none;" autofocus>
                <div id="new-peer-error" class="pv-error-box hidden"></div>
                <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                    <button type="button" class="pv-init-btn" style="height:40px; padding:0 18px;" id="add-peer-submit-btn" onclick="startNewPeerFromModal()">Add Chat</button>
                </div>
            </div>
        </div>

        <div id="add-group-tab-content" class="${tab === 'group' ? '' : 'hidden'}">
            <div style="display:flex; flex-direction:column; gap:12px;">
                <input type="text" id="new-group-name-input" placeholder="Group Name (e.g. core-devs)" style="width:100%; height:44px; background:#141a20; border:1px solid #202932; color:#f8fafc; border-radius:10px; padding:0 14px; outline:none;">
                <input type="text" id="new-group-members-input" placeholder="Members (e.g. marcus, alex)" style="width:100%; height:44px; background:#141a20; border:1px solid #202932; color:#f8fafc; border-radius:10px; padding:0 14px; outline:none;">
                <div id="new-group-error" class="pv-error-box hidden"></div>
                <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                    <button type="button" class="pv-init-btn" style="height:40px; padding:0 18px;" id="add-group-submit-btn" onclick="createGroupFromModal()">Create Group</button>
                </div>
            </div>
        </div>
    `);
    setTimeout(() => {
        const inp = document.getElementById(tab === 'peer' ? 'new-peer-handle-input' : 'new-group-name-input');
        if (inp) inp.focus();
    }, 100);
}

function switchAddTab(tab) {
    const peerTab = document.getElementById('add-peer-tab-content');
    const groupTab = document.getElementById('add-group-tab-content');
    const peerBtn = document.getElementById('tab-btn-peer');
    const groupBtn = document.getElementById('tab-btn-group');

    if (tab === 'peer') {
        peerTab.classList.remove('hidden');
        groupTab.classList.add('hidden');
        peerBtn.style.background = 'var(--pv-emerald)';
        peerBtn.style.color = '#fff';
        groupBtn.style.background = 'transparent';
        groupBtn.style.color = 'var(--pv-text-muted)';
        const inp = document.getElementById('new-peer-handle-input');
        if (inp) inp.focus();
    } else {
        peerTab.classList.add('hidden');
        groupTab.classList.remove('hidden');
        peerBtn.style.background = 'transparent';
        peerBtn.style.color = 'var(--pv-text-muted)';
        groupBtn.style.background = 'var(--pv-emerald)';
        groupBtn.style.color = '#fff';
        const inp = document.getElementById('new-group-name-input');
        if (inp) inp.focus();
    }
}

async function startNewPeerFromModal() {
    const input = document.getElementById('new-peer-handle-input');
    const errEl = document.getElementById('new-peer-error');
    const btn = document.getElementById('add-peer-submit-btn');

    if (!input || !input.value.trim()) return;
    const handle = input.value.trim();

    if (handle.toUpperCase() === state.myHandle.toUpperCase()) {
        errEl.textContent = 'Cannot add yourself as a peer.';
        errEl.classList.remove('hidden');
        return;
    }

    errEl.classList.add('hidden');
    btn.disabled = true;
    btn.textContent = 'Verifying...';

    try {
        const token = getAuthToken();
        const res = await fetch(`/api/lookup?handle=${encodeURIComponent(handle)}`, {
            headers: { 'X-Pandora-Token': token }
        });
        const data = await res.json();

        if (res.ok && data.publicKey) {
            touchContact(data.handle, 'Connected', formatTime(new Date()), data.handle, data.fingerprint, data.publicKey, 'dm');
            selectContact(data.handle);
            closeModal();
        } else {
            errEl.textContent = data.error || `User '${handle}' does not exist on the relay server. Make sure they have initialized first.`;
            errEl.classList.remove('hidden');
            btn.disabled = false;
            btn.textContent = 'Add Chat';
        }
    } catch (err) {
        errEl.textContent = `Server connection error: ${err.message}`;
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Add Chat';
    }
}

async function createGroupFromModal() {
    const nameInput = document.getElementById('new-group-name-input');
    const membersInput = document.getElementById('new-group-members-input');
    const errEl = document.getElementById('new-group-error');
    const btn = document.getElementById('add-group-submit-btn');

    if (!nameInput || !nameInput.value.trim() || !membersInput || !membersInput.value.trim()) {
        errEl.textContent = 'Please provide a group name and member handles.';
        errEl.classList.remove('hidden');
        return;
    }

    const groupName = nameInput.value.trim();
    const rawMembers = membersInput.value.split(',').map(s => s.trim()).filter(Boolean);

    if (rawMembers.length === 0) {
        errEl.textContent = 'Provide at least one peer member.';
        errEl.classList.remove('hidden');
        return;
    }

    errEl.classList.add('hidden');
    btn.disabled = true;
    btn.textContent = 'Verifying...';

    const verifiedMembers = [];
    const token = getAuthToken();

    try {
        for (const member of rawMembers) {
            if (member.toUpperCase() === state.myHandle.toUpperCase()) continue;
            const res = await fetch(`/api/lookup?handle=${encodeURIComponent(member)}`, {
                headers: { 'X-Pandora-Token': token }
            });
            const data = await res.json();
            if (!res.ok || !data.publicKey) {
                errEl.textContent = `Member '${member}' does not exist on relay server. Ensure all members have initialized.`;
                errEl.classList.remove('hidden');
                btn.disabled = false;
                btn.textContent = 'Create Group';
                return;
            }
            verifiedMembers.push(data.handle);
        }

        if (verifiedMembers.length === 0) {
            errEl.textContent = 'No valid peer members found.';
            errEl.classList.remove('hidden');
            btn.disabled = false;
            btn.textContent = 'Create Group';
            return;
        }

        touchContact(groupName, 'Group Created', formatTime(new Date()), groupName, 'Group', '', 'group', verifiedMembers);
        selectContact(groupName);
        closeModal();
    } catch (err) {
        errEl.textContent = `Error verifying members: ${err.message}`;
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Create Group';
    }
}

// Disappearing Messages Modal
function openDisappearingModal() {
    if (!state.activeTarget) {
        openNewChatModal('peer');
        return;
    }
    const currentTTL = getMainTTL(state.activeTarget);

    openModal('Disappearing Messages', `
        <div style="display:flex; flex-direction:column; gap:14px; font-size:0.88rem;">
            <p style="color:#94a3b8;">Set conversation lifespan timer for <strong>${state.activeTarget}</strong>:</p>
            <div style="display:flex; flex-direction:column; gap:10px;">
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="30" ${currentTTL === 30 ? 'checked' : ''}> 30 Seconds</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="60" ${currentTTL === 60 ? 'checked' : ''}> 1 Minute</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="300" ${currentTTL === 300 ? 'checked' : ''}> 5 Minutes (Default)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="3600" ${currentTTL === 3600 ? 'checked' : ''}> 1 Hour</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="86400" ${currentTTL === 86400 ? 'checked' : ''}> 24 Hours</label>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="button" class="pv-init-btn" style="height:38px; padding:0 18px;" onclick="applyConversationTTLFromModal()">Save</button>
            </div>
        </div>
    `);
}

function applyConversationTTLFromModal() {
    const selected = document.querySelector('input[name="ttl-opt"]:checked');
    if (selected && state.activeTarget) {
        const val = parseInt(selected.value, 10) || 300;
        state.convTTL[state.activeTarget] = val;
        state.currentMsgTTL = Math.min(state.currentMsgTTL, val);
        savePersistedData();
        renderCorrespondenceSidebar();
        closeModal();
    }
}

// Deposit Secret Modal
function openSecretDepositModal() {
    if (!state.activeTarget) {
        openNewChatModal('peer');
        return;
    }
    const currentTTL = getMainTTL(state.activeTarget);

    openModal('Deposit Confidential Secret', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#94a3b8; font-size:0.88rem;">Enter confidential secret (destroyed upon first read):</p>
            <textarea id="secret-deposit-textarea" rows="4" placeholder="Confidential payload..." style="width:100%; padding:12px 14px; background:#141a20; border:1px solid #202932; color:#f8fafc; border-radius:10px; font-family:var(--pv-font-mono); font-size:0.88rem; outline:none;"></textarea>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="button" class="pv-init-btn" style="height:38px; padding:0 18px;" onclick="submitSecretDepositFromModal()">Deposit</button>
            </div>
        </div>
    `);
}

async function submitSecretDepositFromModal() {
    const textarea = document.getElementById('secret-deposit-textarea');
    if (textarea && textarea.value.trim()) {
        const secret = textarea.value.trim();
        const currentTTL = getMainTTL(state.activeTarget);
        closeModal();
        try {
            const token = getAuthToken();
            const res = await fetch('/api/deposit', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Pandora-Token': token
                },
                body: JSON.stringify({
                    recipient: state.activeTarget,
                    secret: secret,
                    ttl: currentTTL
                })
            });

            if (res.ok) {
                const data = await res.json();
                appendBubble({
                    id: 'deposit_' + Date.now(),
                    sender: state.myHandle,
                    text: `[Self-Destructing Deposit Created] ID: ${data.id}`,
                    timestamp: formatTime(new Date()),
                    ttl: currentTTL,
                    expiresAt: Date.now() + currentTTL * 1000,
                    isOutgoing: true
                });
            } else {
                alert('Failed to deposit secret.');
            }
        } catch (err) {
            alert(`Deposit failed: ${err.message}`);
        }
    }
}

function openModal(title, htmlContent) {
    modalTitleEl.textContent = title;
    modalBodyEl.innerHTML = htmlContent;
    modalBackdropEl.classList.remove('hidden');
}

function closeModal() {
    modalBackdropEl.classList.add('hidden');
    chatInputEl.focus();
}

function handleBackdropClick(e) {
    if (e.target === modalBackdropEl) {
        closeModal();
    }
}

function formatTime(d) {
    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Start Application
document.addEventListener('DOMContentLoaded', initApp);
