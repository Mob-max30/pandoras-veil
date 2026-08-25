// ==========================================================================
// PANDORA'S VEIL | Secure Secret Relay & Pure Verified Web Interface
// ==========================================================================

const AVATAR_COLORS = [
    'avatar-sand',
    'avatar-sage',
    'avatar-blush',
    'avatar-clay',
    'avatar-charcoal'
];

function getAvatarColor(handle) {
    let hash = 0;
    for (let i = 0; i < handle.length; i++) {
        hash = handle.charCodeAt(i) + ((hash << 5) - hash);
    }
    const idx = Math.abs(hash) % AVATAR_COLORS.length;
    return AVATAR_COLORS[idx];
}

function getInitials(handle) {
    if (!handle) return '?';
    const clean = handle.replace(/^PV-/, '').replace(/^#/, '');
    const parts = clean.split(/[\s_-]+/);
    if (parts.length >= 2) {
        return (parts[0][0] + parts[1][0]).toUpperCase();
    }
    return clean.slice(0, 2).toUpperCase() || 'P';
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
        const savedContacts = localStorage.getItem('pandora_contacts_v4');
        const savedConvs = localStorage.getItem('pandora_conversations_v4');
        const savedTarget = localStorage.getItem('pandora_active_target_v4');
        const savedTTL = localStorage.getItem('pandora_conv_ttl_v4');

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
        localStorage.setItem('pandora_contacts_v4', JSON.stringify(state.contacts));
        localStorage.setItem('pandora_conversations_v4', JSON.stringify(state.conversations));
        localStorage.setItem('pandora_active_target_v4', state.activeTarget);
        localStorage.setItem('pandora_conv_ttl_v4', JSON.stringify(state.convTTL));
    } catch (e) {
        console.warn('Failed to save to localStorage:', e);
    }
}

// DOM Elements
const myHandleEl = document.getElementById('my-handle');
const myFingerprintEl = document.getElementById('my-fingerprint');
const myAvatarEl = document.getElementById('my-avatar');
const chatsListEl = document.getElementById('chats-list');
const activeHeaderAvatarEl = document.getElementById('active-header-avatar');
const activeContactTitleEl = document.getElementById('active-contact-title');
const activeFpSubtleEl = document.getElementById('active-fp-subtle');
const activeStatusTextEl = document.getElementById('active-status-text');
const chatMessagesContainerEl = document.getElementById('chat-messages-container');
const chatMessagesScrollEl = document.getElementById('chat-messages-scroll');
const chatInputEl = document.getElementById('chat-input');
const messageFormEl = document.getElementById('message-form');
const modalBackdropEl = document.getElementById('modal-backdrop');
const modalTitleEl = document.getElementById('modal-title');
const modalBodyEl = document.getElementById('modal-body');
const chatSearchEl = document.getElementById('chat-search');
const initOverlayEl = document.getElementById('init-overlay');
const initHandleInputEl = document.getElementById('init-handle-input');
const initErrorMsgEl = document.getElementById('init-error-msg');
const initSubmitBtnEl = document.getElementById('init-submit-button');
const headerTtlLabelEl = document.getElementById('header-ttl-label');
const currentMsgTtlBadgeEl = document.getElementById('current-msg-ttl-badge');

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

            myHandleEl.textContent = state.myHandle;
            myFingerprintEl.textContent = `FP: ${state.myFingerprint}`;
            myAvatarEl.textContent = getInitials(state.myHandle).charAt(0) || 'U';
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

            myHandleEl.textContent = state.myHandle;
            myFingerprintEl.textContent = `FP: ${state.myFingerprint}`;
            myAvatarEl.textContent = getInitials(state.myHandle).charAt(0) || 'U';

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
    chatsListEl.innerHTML = '';

    if (state.contacts.length === 0) {
        const emptyState = document.createElement('div');
        emptyState.style.cssText = 'padding: 36px 20px; text-align: center; color: #8fa099; font-size: 0.88rem; line-height: 1.6;';
        emptyState.innerHTML = `
            <div>No previous connections.</div>
            <div style="margin-top: 10px; font-size: 0.85rem; color: #c69b59; cursor: pointer; font-weight:600;" onclick="openNewChatModal()">+ Add Correspondence</div>
        `;
        chatsListEl.appendChild(emptyState);

        activeHeaderAvatarEl.textContent = '—';
        activeHeaderAvatarEl.className = 'avatar avatar-sand';
        activeContactTitleEl.textContent = 'No Active Correspondence';
        activeFpSubtleEl.textContent = '';
        activeStatusTextEl.textContent = state.serverConnected ? 'Connected to Relay' : 'Connecting...';
        headerTtlLabelEl.textContent = '5m Lifespan';
        currentMsgTtlBadgeEl.textContent = '5m';
        return;
    }

    state.contacts.forEach(contact => {
        const initials = getInitials(contact.name || contact.handle);
        const colorClass = getAvatarColor(contact.handle);
        const isActive = contact.handle === state.activeTarget;

        const card = document.createElement('div');
        card.className = `chat-card ${isActive ? 'active' : ''}`;
        card.setAttribute('data-handle', contact.handle);

        card.innerHTML = `
            <div class="avatar ${colorClass}">${initials}</div>
            <div class="chat-card-content">
                <div class="chat-card-header">
                    <span class="contact-card-name">${contact.name || contact.handle}</span>
                    <span class="card-time">${contact.time || ''}</span>
                </div>
                <div class="chat-card-footer">
                    <span class="card-snippet">${contact.lastMessage || 'Connected'}</span>
                </div>
            </div>
        `;

        card.addEventListener('click', () => {
            selectContact(contact.handle);
        });

        chatsListEl.appendChild(card);
    });

    // Update active header details
    const activeContact = state.contacts.find(c => c.handle === state.activeTarget);
    if (activeContact) {
        const initials = getInitials(activeContact.name || activeContact.handle);
        const colorClass = getAvatarColor(activeContact.handle);

        activeHeaderAvatarEl.textContent = initials;
        activeHeaderAvatarEl.className = `avatar ${colorClass}`;
        activeContactTitleEl.textContent = activeContact.name || activeContact.handle;
        activeFpSubtleEl.textContent = activeContact.type === 'group' ? `(${activeContact.members.length} Members)` : activeContact.fp;
        activeStatusTextEl.textContent = activeContact.type === 'group' ? 'Group Session' : 'Registered Peer';
        state.isGroup = activeContact.type === 'group';
        state.groupMembers = activeContact.members || [];
        state.activeTargetFP = activeContact.fp || '';
        state.activeTargetPK = activeContact.publicKey || '';

        // Sync TTL controls
        const mainTTL = getMainTTL(state.activeTarget);
        headerTtlLabelEl.textContent = `${formatTTL(mainTTL)} Lifespan`;
        state.currentMsgTTL = Math.min(state.currentMsgTTL, mainTTL);
        currentMsgTtlBadgeEl.textContent = formatTTL(state.currentMsgTTL);
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

function cycleMessageTTL() {
    if (!state.activeTarget) return;
    const mainTTL = getMainTTL(state.activeTarget);
    const intervals = [30, 60, 300, 1800, 3600, 86400, 604800].filter(t => t <= mainTTL);
    if (intervals.length === 0) intervals.push(mainTTL);

    let idx = intervals.indexOf(state.currentMsgTTL);
    if (idx === -1 || idx >= intervals.length - 1) {
        state.currentMsgTTL = intervals[0];
    } else {
        state.currentMsgTTL = intervals[idx + 1];
    }
    currentMsgTtlBadgeEl.textContent = formatTTL(state.currentMsgTTL);
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
        if (!state.activeTarget) {
            activeStatusTextEl.textContent = 'Connected to Relay';
        }
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
                    appendLinenBubble(msg);
                }
            }
        } catch (err) {
            console.error('Error parsing stream event:', err);
        }
    };

    state.eventSource.onerror = () => {
        state.serverConnected = false;
        if (!state.activeTarget) {
            activeStatusTextEl.textContent = 'Reconnecting to Relay...';
        }
    };
}

// 4. Render Active Conversation Messages
function renderActiveConversation() {
    if (!state.activeTarget || state.contacts.length === 0) {
        chatMessagesContainerEl.innerHTML = `
            <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; padding-top: 120px; text-align:center; color:#78817d;">
                <h3 style="font-family:var(--font-serif); font-size:1.6rem; color:var(--linen-card-green); margin-bottom:8px; font-weight:400;">Pandora's Veil</h3>
                <p style="font-size:0.92rem; max-width:360px; color:#636765; line-height:1.5;">Zero-knowledge cryptographic secret relay. Add a peer to begin correspondence.</p>
                <button class="linen-btn" style="margin-top:20px; padding:10px 22px; font-size:0.88rem;" onclick="openNewChatModal()">+ Add Correspondence</button>
            </div>
        `;
        return;
    }

    const separatorHTML = `
        <div class="linen-date-separator">
            <span class="sep-line"></span>
            <span class="sep-label">TODAY</span>
            <span class="sep-line"></span>
        </div>
    `;

    chatMessagesContainerEl.innerHTML = separatorHTML;

    const msgs = state.conversations[state.activeTarget] || [];
    const now = Date.now();
    const validMsgs = msgs.filter(m => !m.expiresAt || m.expiresAt > now);
    state.conversations[state.activeTarget] = validMsgs;

    validMsgs.forEach(msg => appendLinenBubble(msg));
    scrollChatToBottom();
}

// 5. Append Linen Message Bubble
function appendLinenBubble(msg) {
    const groupEl = document.createElement('div');
    groupEl.id = msg.id || ('msg_' + Date.now());
    groupEl.className = `linen-bubble-group ${msg.isOutgoing ? 'outgoing-group' : 'incoming-group'}`;

    const cardEl = document.createElement('div');
    cardEl.className = `linen-card ${msg.isOutgoing ? 'outgoing-card' : 'incoming-card'}`;

    const pEl = document.createElement('p');
    pEl.textContent = msg.text;
    cardEl.appendChild(pEl);
    groupEl.appendChild(cardEl);

    if (msg.timestamp) {
        const timeSpan = document.createElement('span');
        timeSpan.className = 'bubble-time-stamp';
        const ttlNote = msg.ttl ? ` • ${formatTTL(msg.ttl)}` : '';
        timeSpan.textContent = `${msg.timestamp}${ttlNote}`;
        groupEl.appendChild(timeSpan);
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
        openNewChatModal();
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
    appendLinenBubble(msg);

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
            appendLinenBubble({
                id: 'err_' + Date.now(),
                sender: 'SYSTEM',
                text: `Delivery failed: ${errData.error || 'Relay error'}`,
                timestamp: formatTime(new Date()),
                isOutgoing: false
            });
        }
    } catch (err) {
        appendLinenBubble({
            id: 'err_' + Date.now(),
            sender: 'SYSTEM',
            text: `Network error: ${err.message}`,
            timestamp: formatTime(new Date()),
            isOutgoing: false
        });
    }
}

// 7. Expired Message Pruner (Deletes messages locally after their TTL expires)
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

// 8. Setup Event Listeners
function setupEventListeners() {
    messageFormEl.addEventListener('submit', handleSendMessage);

    chatSearchEl.addEventListener('input', (e) => {
        const query = e.target.value.toLowerCase();
        document.querySelectorAll('.chat-card').forEach(card => {
            const name = card.querySelector('.contact-card-name').textContent.toLowerCase();
            if (name.includes(query)) {
                card.style.display = 'flex';
            } else {
                card.style.display = 'none';
            }
        });
    });
}

// 9. Modals & Account Management

// Profile Modal (Matches `pv identity` format + Delete Account option)
function toggleProfileModal() {
    openModal('Device Identity (Verified on Relay)', `
        <div class="identity-credential-box">
            <div class="cred-row">
                <span class="cred-label">Handle</span>
                <span class="cred-value">${state.myHandle}</span>
            </div>
            <div class="cred-row">
                <span class="cred-label">Device Fingerprint</span>
                <span class="cred-value" style="color:var(--linen-card-green); font-weight:600;">${state.myFingerprint}</span>
            </div>
            <div class="cred-row">
                <span class="cred-label">Public Key (age / X25519)</span>
                <span class="cred-value" style="font-size:0.78rem;">${state.myPublicKey}</span>
            </div>
            <div class="cred-row">
                <span class="cred-label">Security Property</span>
                <span style="font-size:0.82rem; color:#47554f;">Private key is secured locally on disk (~/.pandora/identity.json, 0600 permissions).</span>
            </div>
        </div>
        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:20px; padding-top:16px; border-top:1px solid #dfd8cc;">
            <span style="font-size:0.82rem; color:#78807c;">Reset or destroy local credentials:</span>
            <button type="button" class="linen-btn-danger" onclick="confirmDeleteAccount()">Delete Account</button>
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

// Peer Contact Details Modal (Matches `pv identity` format + Remove Correspondence option)
function openContactDetailsModal() {
    if (!state.activeTarget) return;
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;
    const fp = contact ? (contact.fp || state.activeTargetFP || 'Verified') : 'Verified';
    const pk = contact ? (contact.publicKey || state.activeTargetPK || 'Verified on Relay') : 'Verified on Relay';

    if (contact && contact.type === 'group') {
        const memberList = (contact.members || []).map(m => `<li style="padding:4px 0;"><code>${m}</code></li>`).join('');
        openModal(`Group: ${displayName}`, `
            <div class="identity-credential-box">
                <div class="cred-row">
                    <span class="cred-label">Group Name</span>
                    <span class="cred-value">${displayName}</span>
                </div>
                <div class="cred-row">
                    <span class="cred-label">Verified Group Members (${contact.members.length})</span>
                    <ul style="margin:4px 0 0 18px; font-size:0.88rem; color:#222725;">${memberList}</ul>
                </div>
                <div class="cred-row">
                    <span class="cred-label">Encryption Suite</span>
                    <span class="cred-value">age / X25519 Multi-Recipient Delivery</span>
                </div>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:20px;">
                <button type="button" class="linen-btn-danger" onclick="removeCorrespondence('${state.activeTarget}')">Remove Group</button>
            </div>
        `);
        return;
    }

    openModal(`Peer Identity: ${displayName}`, `
        <div class="identity-credential-box">
            <div class="cred-row">
                <span class="cred-label">Handle</span>
                <span class="cred-value">${state.activeTarget}</span>
            </div>
            <div class="cred-row">
                <span class="cred-label">Device Fingerprint</span>
                <span class="cred-value" style="color:var(--linen-card-green); font-weight:600;">${fp}</span>
            </div>
            <div class="cred-row">
                <span class="cred-label">Public Key</span>
                <span class="cred-value" style="font-size:0.78rem;">${pk}</span>
            </div>
            <div class="cred-row">
                <span class="cred-label">Relay Status</span>
                <span class="cred-value" style="color:var(--linen-card-green);">Verified & Active on Relay</span>
            </div>
        </div>
        <div style="display:flex; justify-content:flex-end; margin-top:20px;">
            <button type="button" class="linen-btn-danger" onclick="removeCorrespondence('${state.activeTarget}')">Remove Correspondence</button>
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

// Add Correspondence Modal (1-on-1 Peer or Encrypted Group)
function openNewChatModal() {
    openModal('Add Correspondence', `
        <div class="modal-tab-row">
            <button type="button" class="modal-tab-btn active" id="tab-btn-peer" onclick="switchAddTab('peer')">1-on-1 Peer</button>
            <button type="button" class="modal-tab-btn" id="tab-btn-group" onclick="switchAddTab('group')">Encrypted Group</button>
        </div>

        <!-- 1-on-1 Peer Tab -->
        <div id="add-peer-tab-content">
            <div style="display:flex; flex-direction:column; gap:12px;">
                <p style="color:#636765; font-size:0.88rem;">Enter the exact peer handle to verify against the relay server:</p>
                <input type="text" id="new-peer-handle-input" placeholder="e.g. PV-BOB or ALICE" style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-size:0.95rem; outline:none;" autofocus>
                <div id="new-peer-error" class="init-error-text hidden"></div>
                <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:8px;">
                    <button type="button" class="linen-btn" id="add-peer-submit-btn" onclick="startNewPeerFromModal()">Verify & Add Peer</button>
                </div>
            </div>
        </div>

        <!-- Encrypted Group Tab -->
        <div id="add-group-tab-content" class="hidden">
            <div style="display:flex; flex-direction:column; gap:12px;">
                <p style="color:#636765; font-size:0.88rem;">Create a multi-recipient encrypted group:</p>
                <input type="text" id="new-group-name-input" placeholder="Group Name (e.g. Development)" style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-size:0.95rem; outline:none;">
                <input type="text" id="new-group-members-input" placeholder="Members (e.g. PV-BOB, PV-ALICE, PV-PRANAV)" style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-size:0.95rem; outline:none;">
                <div id="new-group-error" class="init-error-text hidden"></div>
                <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:8px;">
                    <button type="button" class="linen-btn" id="add-group-submit-btn" onclick="createGroupFromModal()">Verify & Create Group</button>
                </div>
            </div>
        </div>
    `);
    setTimeout(() => {
        const inp = document.getElementById('new-peer-handle-input');
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
        peerBtn.classList.add('active');
        groupBtn.classList.remove('active');
        const inp = document.getElementById('new-peer-handle-input');
        if (inp) inp.focus();
    } else {
        peerTab.classList.add('hidden');
        groupTab.classList.remove('hidden');
        peerBtn.classList.remove('active');
        groupBtn.classList.add('active');
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
    btn.textContent = 'Verifying on Relay...';

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
            btn.textContent = 'Verify & Add Peer';
        }
    } catch (err) {
        errEl.textContent = `Server connection error: ${err.message}`;
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Verify & Add Peer';
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
    btn.textContent = 'Verifying Members on Relay...';

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
                btn.textContent = 'Verify & Create Group';
                return;
            }
            verifiedMembers.push(data.handle);
        }

        if (verifiedMembers.length === 0) {
            errEl.textContent = 'No valid peer members found.';
            errEl.classList.remove('hidden');
            btn.disabled = false;
            btn.textContent = 'Verify & Create Group';
            return;
        }

        touchContact(groupName, 'Group Created', formatTime(new Date()), groupName, 'Group', '', 'group', verifiedMembers);
        selectContact(groupName);
        closeModal();
    } catch (err) {
        errEl.textContent = `Error verifying members: ${err.message}`;
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Verify & Create Group';
    }
}

// Disappearing Messages Timer Modal
function openDisappearingModal() {
    if (!state.activeTarget) {
        openNewChatModal();
        return;
    }
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;
    const currentTTL = getMainTTL(state.activeTarget);

    openModal('Disappearing Messages Lifespan', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Set conversation lifespan timer for <strong>${displayName}</strong>. Messages expire on the server and are automatically wiped everywhere after this duration:</p>
            <div style="display:flex; flex-direction:column; gap:10px; margin:4px 0;">
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="30" ${currentTTL === 30 ? 'checked' : ''}> 30 Seconds</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="60" ${currentTTL === 60 ? 'checked' : ''}> 1 Minute</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="300" ${currentTTL === 300 ? 'checked' : ''}> 5 Minutes (Default)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="3600" ${currentTTL === 3600 ? 'checked' : ''}> 1 Hour</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="86400" ${currentTTL === 86400 ? 'checked' : ''}> 24 Hours</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="604800" ${currentTTL === 604800 ? 'checked' : ''}> 7 Days</label>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="button" class="linen-btn" onclick="applyConversationTTLFromModal()">Save Lifespan</button>
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
        openNewChatModal();
        return;
    }
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;
    const currentTTL = getMainTTL(state.activeTarget);

    openModal('Deposit Confidential Secret (GETDEL)', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Enter confidential secret (passwords, tokens) for <strong>${displayName}</strong>:</p>
            <textarea id="secret-deposit-textarea" rows="4" placeholder="Confidential payload..." style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-family:var(--font-mono); font-size:0.88rem; outline:none;"></textarea>
            <div style="display:flex; justify-content:space-between; align-items:center; margin-top:8px;">
                <span style="font-size:0.8rem; color:var(--linen-terracotta);">Burn-After-Reading: ON (${formatTTL(currentTTL)} Lifespan)</span>
                <button type="button" class="linen-btn" onclick="submitSecretDepositFromModal()">Deposit Secret</button>
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
                appendLinenBubble({
                    id: 'deposit_' + Date.now(),
                    sender: state.myHandle,
                    text: `[Self-Destructing Deposit Created] ID: ${data.id} (Destroyed upon first read by recipient)`,
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
