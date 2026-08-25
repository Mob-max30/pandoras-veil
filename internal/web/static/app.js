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
    if (seconds === 60) return '60s';
    if (!seconds || seconds <= 0) return '5m';
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
    return `${Math.floor(seconds / 86400)}d`;
}

function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
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
    defaultTTL: 300,     // Global default lifespan (default 300s / 5m)
    convTTL: {},        // Main Disappearing TTL per conversation
    customMsgTTL: 0,    // 0 means inherit main TTL; >0 means independent custom TTL
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
        const savedContacts = localStorage.getItem('pandora_contacts_v6');
        const savedConvs = localStorage.getItem('pandora_conversations_v6');
        const savedTarget = localStorage.getItem('pandora_active_target_v6');
        const savedTTL = localStorage.getItem('pandora_conv_ttl_v6');
        const savedDefaultTTL = localStorage.getItem('pandora_default_ttl_v6');

        state.contacts = savedContacts ? JSON.parse(savedContacts) : [];
        // Sanitize any empty or invalid contact entries
        state.contacts = state.contacts.filter(c => c && c.handle && c.handle.trim() !== '');
        state.conversations = savedConvs ? JSON.parse(savedConvs) : {};
        state.convTTL = savedTTL ? JSON.parse(savedTTL) : {};
        state.defaultTTL = savedDefaultTTL ? parseInt(savedDefaultTTL, 10) : 300;

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
        state.defaultTTL = 300;
        state.activeTarget = '';
    }
}

function savePersistedData() {
    try {
        localStorage.setItem('pandora_contacts_v6', JSON.stringify(state.contacts));
        localStorage.setItem('pandora_conversations_v6', JSON.stringify(state.conversations));
        localStorage.setItem('pandora_active_target_v6', state.activeTarget);
        localStorage.setItem('pandora_conv_ttl_v6', JSON.stringify(state.convTTL));
        localStorage.setItem('pandora_default_ttl_v6', String(state.defaultTTL));
    } catch (e) {
        console.warn('Failed to save to localStorage:', e);
    }
}

// DOM Elements
const myHandleLabelEl = document.getElementById('my-handle-label');
const myFingerprintEl = document.getElementById('my-fingerprint');
const directChatsListEl = document.getElementById('direct-chats-list');
const groupChatsListEl = document.getElementById('group-chats-list');
const activeHeaderAvatarEl = document.getElementById('active-header-avatar');
const activeContactTitleEl = document.getElementById('active-contact-title');
const activeStatusLineEl = document.getElementById('active-status-line');
const activeStatusTextEl = document.getElementById('active-status-text');
const headerConnStatusEl = document.getElementById('header-conn-status');
const topMainTtlLabelEl = document.getElementById('top-main-ttl-label');
const currentMsgTtlBadgeEl = document.getElementById('current-msg-ttl-badge');
const msgTtlBtnEl = document.getElementById('msg-ttl-btn');
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

            if (myHandleLabelEl) myHandleLabelEl.textContent = state.myHandle;
            if (myFingerprintEl) myFingerprintEl.textContent = state.myFingerprint;
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
    const mainTTL = getMainTTL(state.activeTarget);

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

    if (topMainTtlLabelEl) {
        topMainTtlLabelEl.textContent = `${formatTTL(mainTTL)} Lifespan`;
    }
    updateMsgTTLBadge();
}

function getMainTTL(handle) {
    if (handle && state.convTTL[handle]) {
        return state.convTTL[handle];
    }
    return state.defaultTTL || 300;
}

function updateMsgTTLBadge() {
    if (!currentMsgTtlBadgeEl) return;
    const mainTTL = getMainTTL(state.activeTarget);
    const effective = state.customMsgTTL > 0 ? state.customMsgTTL : mainTTL;
    currentMsgTtlBadgeEl.textContent = formatTTL(effective);
    if (state.customMsgTTL > 0 && state.customMsgTTL !== mainTTL) {
        if (msgTtlBtnEl) msgTtlBtnEl.classList.add('active-custom');
    } else {
        if (msgTtlBtnEl) msgTtlBtnEl.classList.remove('active-custom');
    }
}

function cycleMessageTTL() {
    const options = [60, 300, 3600, 86400];
    const mainTTL = getMainTTL(state.activeTarget);
    const current = state.customMsgTTL > 0 ? state.customMsgTTL : mainTTL;
    let idx = options.indexOf(current);
    if (idx === -1 || idx >= options.length - 1) {
        state.customMsgTTL = options[0];
    } else {
        state.customMsgTTL = options[idx + 1];
    }
    updateMsgTTLBadge();
}

function selectContact(handle) {
    state.activeTarget = handle;
    state.customMsgTTL = 0; // reset to inherit main time
    savePersistedData();
    renderCorrespondenceSidebar();
    renderActiveConversation();
    chatInputEl.focus();
}

function touchContact(handle, lastMsgText, timestamp, senderName, fp, pk, type, members) {
    if (!handle || typeof handle !== 'string' || !handle.trim()) return;
    handle = handle.trim();
    const displayName = (senderName && typeof senderName === 'string' && senderName.trim()) ? senderName.trim() : handle;

    let contact = state.contacts.find(c => c.handle === handle);
    if (!contact) {
        contact = {
            handle: handle,
            name: displayName,
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
        if (displayName) contact.name = displayName;
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

                let isFile = false;
                let fileName = '';
                let fileSize = '';
                let fileData = '';
                let previewText = data.text;

                try {
                    if (typeof data.text === 'string' && data.text.startsWith('{"__pv_file":true')) {
                        const fileObj = JSON.parse(data.text);
                        isFile = true;
                        fileName = fileObj.name || 'attachment';
                        fileSize = fileObj.size || '';
                        fileData = fileObj.data || '';
                        previewText = `📎 ${fileName}`;
                    }
                } catch (e) {}

                const msg = {
                    id: 'msg_' + Date.now() + '_' + Math.random().toString(36).substr(2, 5),
                    sender: senderName,
                    text: previewText,
                    isFile: isFile,
                    fileName: fileName,
                    fileSize: fileSize,
                    fileData: fileData,
                    timestamp: timestamp,
                    ttl: msgTTL,
                    expiresAt: Date.now() + msgTTL * 1000,
                    isOutgoing: false
                };

                if (!state.conversations[senderName]) {
                    state.conversations[senderName] = [];
                }
                state.conversations[senderName].push(msg);

                touchContact(senderName, previewText, timestamp, senderName);

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
    // Auto-detect JSON file payloads in msg.text if not already flagged
    if (!msg.isFile && typeof msg.text === 'string' && (msg.text.includes('"__pv_file"') || msg.text.includes('"is_file"'))) {
        try {
            const parsed = JSON.parse(msg.text);
            if (parsed.__pv_file || parsed.is_file) {
                msg.isFile = true;
                msg.fileName = parsed.name || parsed.filename || 'attachment';
                msg.fileSize = parsed.size || '';
                msg.fileType = parsed.type || '';
                msg.fileData = parsed.data || (parsed.data_b64 ? `data:application/octet-stream;base64,${parsed.data_b64}` : '');
            }
        } catch (e) {}
    }

    const groupEl = document.createElement('div');
    groupEl.id = msg.id || ('msg_' + Date.now());
    groupEl.className = `pv-bubble-group ${msg.isOutgoing ? 'outgoing' : 'incoming'}`;

    if (msg.isFile) {
        const isImage = (msg.fileType && msg.fileType.startsWith('image/')) ||
                        (msg.fileName && msg.fileName.match(/\.(jpeg|jpg|png|gif|webp|svg|bmp)$/i));

        const fileCard = document.createElement('div');
        fileCard.className = 'pv-file-card';
        fileCard.style.cursor = msg.fileData ? 'pointer' : 'default';

        if (isImage && msg.fileData) {
            fileCard.style.flexDirection = 'column';
            fileCard.style.alignItems = 'flex-start';
            fileCard.style.padding = '10px';
            fileCard.style.maxWidth = '300px';

            fileCard.innerHTML = `
                <div style="width:100%; max-height:220px; overflow:hidden; border-radius:10px; margin-bottom:8px; background:#07120e; display:flex; align-items:center; justify-content:center;">
                    <img src="${msg.fileData}" alt="${msg.fileName || 'image'}" style="max-width:100%; max-height:220px; object-fit:contain; border-radius:8px; display:block;">
                </div>
                <div style="display:flex; align-items:center; justify-content:space-between; width:100%; gap:8px;">
                    <div class="pv-file-meta" style="flex:1; overflow:hidden;">
                        <div class="pv-file-name" style="font-size:0.86rem;">${msg.fileName || 'image.jpeg'}</div>
                        <div class="pv-file-size" style="font-size:0.74rem;">${msg.fileSize || 'Image file'}</div>
                    </div>
                    <div class="pv-file-status" style="font-weight:700; color:var(--pv-emerald-light); font-size:0.8rem; flex-shrink:0;">⬇ Download</div>
                </div>
            `;
        } else {
            fileCard.innerHTML = `
                <div class="pv-file-icon-box">
                    <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>
                </div>
                <div class="pv-file-meta">
                    <div class="pv-file-name">${msg.fileName || 'file'}</div>
                    <div class="pv-file-size">${msg.fileSize || 'Attachment'}</div>
                    <div class="pv-file-status">${msg.fileData ? '⬇ Click to download' : '✓ Transmitted'}</div>
                </div>
            `;
        }

        if (msg.fileData) {
            fileCard.onclick = () => {
                const a = document.createElement('a');
                a.href = msg.fileData;
                a.download = msg.fileName || 'attachment';
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
            };
        }
        groupEl.appendChild(fileCard);

        const metaEl = document.createElement('div');
        metaEl.className = 'pv-bubble-meta';
        const ttlLabel = msg.ttl ? ` • ${formatTTL(msg.ttl)}` : '';
        metaEl.textContent = (msg.isOutgoing ? 'File Encrypted & Dispatched' : 'File Decrypted locally') + ttlLabel;
        groupEl.appendChild(metaEl);
    } else {
        const cardEl = document.createElement('div');
        cardEl.className = 'pv-card-bubble';
        cardEl.textContent = msg.text;
        groupEl.appendChild(cardEl);

        const metaEl = document.createElement('div');
        metaEl.className = 'pv-bubble-meta';
        const ttlLabel = msg.ttl ? ` • ${formatTTL(msg.ttl)}` : '';
        metaEl.textContent = (msg.isOutgoing ? 'Encrypted & Dispatched' : 'Decrypted locally') + ttlLabel;
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
    // Use custom independent message TTL if set (> 0), otherwise use main conversation TTL
    const effectiveTTL = state.customMsgTTL > 0 ? state.customMsgTTL : getMainTTL(state.activeTarget);
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

// 6b. Native File Attachment Selection & Dispatch
function triggerFileInput() {
    if (!state.activeTarget) {
        openNewChatModal();
        return;
    }
    const fileInput = document.getElementById('file-attachment-input');
    if (fileInput) {
        fileInput.value = '';
        fileInput.click();
    }
}

async function handleFileSelected(event) {
    const file = event.target.files && event.target.files[0];
    if (!file || !state.activeTarget) return;

    const reader = new FileReader();
    reader.onload = async (e) => {
        const base64Data = e.target.result;
        const filePayload = JSON.stringify({
            __pv_file: true,
            name: file.name,
            size: formatBytes(file.size),
            type: file.type || 'application/octet-stream',
            data: base64Data
        });

        const timestamp = formatTime(new Date());
        const effectiveTTL = state.customMsgTTL > 0 ? state.customMsgTTL : getMainTTL(state.activeTarget);
        const msgId = 'msg_' + Date.now() + '_' + Math.random().toString(36).substr(2, 5);

        const msg = {
            id: msgId,
            sender: state.myHandle,
            isFile: true,
            fileName: file.name,
            fileSize: formatBytes(file.size),
            fileData: base64Data,
            text: `📎 ${file.name}`,
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

        touchContact(state.activeTarget, `📎 ${file.name}`, timestamp);

        try {
            const token = getAuthToken();
            const payload = {
                target: state.activeTarget,
                isGroup: state.isGroup,
                groupMembers: state.groupMembers,
                text: filePayload,
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
                    text: `File delivery failed: ${errData.error || 'Relay error'}`,
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
    };
    reader.readAsDataURL(file);
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

// Add New Chat Modal
function openNewChatModal() {
    openModal('New Chat', `
        <form onsubmit="event.preventDefault(); startNewPeerFromModal();" style="display:flex; flex-direction:column; gap:14px;">
            <input type="text" id="new-peer-handle-input" placeholder="Peer handle (e.g. marcus or alex)" style="width:100%; height:46px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:12px; padding:0 16px; font-size:0.92rem; outline:none;" autofocus>
            <div id="new-peer-error" class="pv-error-box hidden"></div>
            <div style="display:flex; justify-content:flex-end; margin-top:4px;">
                <button type="submit" class="pv-init-btn" style="height:42px; padding:0 22px;" id="add-peer-submit-btn">Start Chat</button>
            </div>
        </form>
    `);
    setTimeout(() => {
        const inp = document.getElementById('new-peer-handle-input');
        if (inp) inp.focus();
    }, 100);
}

// Add New Group Modal
function openNewGroupModal() {
    openModal('New Group', `
        <form onsubmit="event.preventDefault(); createGroupFromModal();" style="display:flex; flex-direction:column; gap:14px;">
            <input type="text" id="new-group-name-input" placeholder="Group Name (e.g. core-devs)" style="width:100%; height:46px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:12px; padding:0 16px; font-size:0.92rem; outline:none;" autofocus>
            <input type="text" id="new-group-members-input" placeholder="Members (e.g. marcus, alex)" style="width:100%; height:46px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:12px; padding:0 16px; font-size:0.92rem; outline:none;">
            <div id="new-group-error" class="pv-error-box hidden"></div>
            <div style="display:flex; justify-content:flex-end; margin-top:4px;">
                <button type="submit" class="pv-init-btn" style="height:42px; padding:0 22px;" id="add-group-submit-btn">Create Group</button>
            </div>
        </form>
    `);
    setTimeout(() => {
        const inp = document.getElementById('new-group-name-input');
        if (inp) inp.focus();
    }, 100);
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
            const peerHandle = (data.handle && data.handle.trim()) ? data.handle.trim() : handle;
            touchContact(peerHandle, 'Connected', formatTime(new Date()), peerHandle, data.fingerprint, data.publicKey, 'dm');
            selectContact(peerHandle);
            closeModal();
        } else {
            errEl.textContent = data.error || `User '${handle}' does not exist on the relay server. Make sure they have initialized first.`;
            errEl.classList.remove('hidden');
            btn.disabled = false;
            btn.textContent = 'Start Chat';
        }
    } catch (err) {
        errEl.textContent = `Server connection error: ${err.message}`;
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Start Chat';
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
    const currentTTL = getMainTTL(state.activeTarget);

    openModal('Disappearing Messages', `
        <form onsubmit="event.preventDefault(); applyConversationTTLFromModal();" style="display:flex; flex-direction:column; gap:16px; font-size:0.9rem;">
            <p style="color:#94a3b8; font-size:0.86rem;">Select lifespan after which messages will automatically burn:</p>
            <div style="display:flex; flex-direction:column; gap:10px;">
                <label style="cursor:pointer; display:flex; align-items:center; gap:10px; color:#f8fafc;"><input type="radio" name="ttl-opt" value="60" ${currentTTL === 60 ? 'checked' : ''}> 60s (1 Minute)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:10px; color:#f8fafc;"><input type="radio" name="ttl-opt" value="300" ${currentTTL === 300 ? 'checked' : ''}> 5m (5 Minutes)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:10px; color:#f8fafc;"><input type="radio" name="ttl-opt" value="3600" ${currentTTL === 3600 ? 'checked' : ''}> 1h (1 Hour)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:10px; color:#f8fafc;"><input type="radio" name="ttl-opt" value="86400" ${currentTTL === 86400 ? 'checked' : ''}> 24h (24 Hours)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:10px; color:#34d399; margin-top:4px;"><input type="checkbox" id="burn-after-read-chk" ${state.burnAfterReading ? 'checked' : ''}> Burn after reading (One-time view)</label>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="submit" class="pv-init-btn" style="height:40px; padding:0 22px;">Save Lifespan</button>
            </div>
        </form>
    `);
}

function applyConversationTTLFromModal() {
    const selected = document.querySelector('input[name="ttl-opt"]:checked');
    const burnChk = document.getElementById('burn-after-read-chk');
    if (burnChk) {
        state.burnAfterReading = burnChk.checked;
    }
    if (selected) {
        const val = parseInt(selected.value, 10) || 300;
        state.defaultTTL = val;
        if (state.activeTarget) {
            state.convTTL[state.activeTarget] = val;
        }
        state.customMsgTTL = 0; // reset message-specific override to match the newly applied main time

        // update top main TTL label
        if (topMainTtlLabelEl) {
            topMainTtlLabelEl.textContent = `${formatTTL(val)} Lifespan`;
        }

        savePersistedData();
        renderCorrespondenceSidebar();
        updateMsgTTLBadge();
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
        const currentTTL = state.customMsgTTL > 0 ? state.customMsgTTL : getMainTTL(state.activeTarget);
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
