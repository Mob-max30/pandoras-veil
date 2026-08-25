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

function formatTime(date) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function escapeHTML(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
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
    defaultTTL: 300,
    convTTL: {},
    customMsgTTL: 0,
    burnAfterReading: true,
    eventSource: null,
    contacts: [],
    conversations: {},
    serverConnected: false
};

const seenMessageIDs = new Set();

function getAuthToken() {
    const meta = document.querySelector('meta[name="pandora-token"]');
    return meta ? meta.getAttribute('content') : '';
}

// 0. Pure Server-Side Session Synchronization (Zero Browser LocalStorage)
let saveDebounceTimer = null;
function syncServerSession() {
    clearTimeout(saveDebounceTimer);
    saveDebounceTimer = setTimeout(async () => {
        try {
            const token = getAuthToken();
            await fetch('/api/conversations', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Pandora-Token': token
                },
                body: JSON.stringify({
                    contacts: state.contacts,
                    conversations: state.conversations,
                    mainTTLs: state.convTTL
                })
            });
        } catch (e) {
            console.warn('Session sync warning:', e);
        }
    }, 300);
}

async function loadServerSession() {
    try {
        const token = getAuthToken();
        const res = await fetch('/api/conversations', {
            headers: { 'X-Pandora-Token': token }
        });
        if (res.ok) {
            const data = await res.json();
            state.contacts = data.contacts || [];
            state.conversations = data.conversations || {};
            state.convTTL = data.mainTTLs || {};

            // Re-populate seenMessageIDs from session history
            Object.values(state.conversations).forEach(msgs => {
                if (Array.isArray(msgs)) {
                    msgs.forEach(m => {
                        if (m && m.id) seenMessageIDs.add(m.id);
                    });
                }
            });

            if (state.contacts.length > 0 && !state.activeTarget) {
                state.activeTarget = state.contacts[0].handle;
            }
        }
    } catch (e) {
        console.warn('Failed to load server session:', e);
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

    await loadServerSession();
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

            if (myHandleLabelEl) myHandleLabelEl.textContent = state.myHandle;
            if (myFingerprintEl) myFingerprintEl.textContent = state.myFingerprint;

            hideInitOverlay();
            await loadServerSession();
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

// 2. Render Correspondence List in Sidebar (Case-Insensitive & Deduplicated)
function renderCorrespondenceSidebar() {
    directChatsListEl.innerHTML = '';
    groupChatsListEl.innerHTML = '';

    const directContacts = state.contacts.filter(c => !c.isGroup && !c.handle.startsWith('#'));
    const groupContacts = state.contacts.filter(c => c.isGroup || c.handle.startsWith('#'));

    // Render Direct Contacts
    if (directContacts.length === 0) {
        const emptyEl = document.createElement('div');
        emptyEl.style.cssText = 'padding: 8px 10px; font-size: 0.8rem; color: #64748b;';
        emptyEl.textContent = 'No direct chats';
        directChatsListEl.appendChild(emptyEl);
    } else {
        directContacts.forEach(contact => {
            const initials = getInitials(contact.displayName || contact.handle);
            const isActive = contact.handle.toLowerCase() === state.activeTarget.toLowerCase();

            const card = document.createElement('div');
            card.className = `pv-contact-card ${isActive ? 'active' : ''}`;
            card.setAttribute('data-handle', contact.handle);

            card.innerHTML = `
                <div class="card-avatar">${initials}</div>
                <span class="card-name">${escapeHTML(contact.displayName || contact.handle)}</span>
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
            const isActive = contact.handle.toLowerCase() === state.activeTarget.toLowerCase();
            const card = document.createElement('div');
            card.className = `pv-group-card ${isActive ? 'active' : ''}`;
            card.setAttribute('data-handle', contact.handle);
            card.textContent = contact.displayName || contact.handle;

            card.addEventListener('click', () => {
                selectContact(contact.handle);
            });

            groupChatsListEl.appendChild(card);
        });
    }

    // Update active header details
    const activeContact = state.contacts.find(c => c.handle.toLowerCase() === state.activeTarget.toLowerCase());
    const mainTTL = getMainTTL(state.activeTarget);

    if (activeContact) {
        const initials = getInitials(activeContact.displayName || activeContact.handle);
        activeHeaderAvatarEl.textContent = initials;
        activeContactTitleEl.textContent = activeContact.displayName || activeContact.handle;
        state.isGroup = activeContact.isGroup || activeContact.handle.startsWith('#');
        state.groupMembers = activeContact.members || [];
        state.activeTargetFP = activeContact.fingerprint || '';
        state.activeTargetPK = activeContact.publicKey || '';

        activeStatusTextEl.textContent = state.isGroup 
            ? `${(state.groupMembers.length > 0 ? state.groupMembers.length : 1)} Members · End-to-end verified` 
            : 'Online · End-to-end verified';
    } else if (state.activeTarget) {
        const initials = getInitials(state.activeTarget);
        activeHeaderAvatarEl.textContent = initials;
        activeContactTitleEl.textContent = state.activeTarget;
        state.isGroup = state.activeTarget.startsWith('#');
        activeStatusTextEl.textContent = 'Online · End-to-end verified';
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
    state.customMsgTTL = 0;
    renderCorrespondenceSidebar();
    renderActiveConversation();
    if (chatInputEl) chatInputEl.focus();
}

// Case-Insensitive Touch Contact with Deduplication
function touchContact(rawHandle, lastMsgText, timestamp, rawDisplayName = '', isGroup = false, groupMembers = [], fp = '', pk = '') {
    if (!rawHandle || typeof rawHandle !== 'string' || !rawHandle.trim()) return;
    const handle = rawHandle.trim();
    const displayName = (rawDisplayName && typeof rawDisplayName === 'string' && rawDisplayName.trim()) ? rawDisplayName.trim() : handle;
    const norm = handle.toLowerCase();

    let contact = state.contacts.find(c => c.handle.toLowerCase() === norm);
    if (!contact) {
        contact = {
            handle: handle,
            displayName: displayName,
            fingerprint: fp || 'Verified',
            publicKey: pk || '',
            isGroup: isGroup || handle.startsWith('#'),
            members: groupMembers || [],
            lastMessage: lastMsgText || '',
            lastTime: timestamp || formatTime(new Date())
        };
        state.contacts.unshift(contact);
    } else {
        contact.lastMessage = lastMsgText || contact.lastMessage;
        contact.lastTime = timestamp || contact.lastTime;
        if (displayName) contact.displayName = displayName;
        if (groupMembers && groupMembers.length > 0) contact.members = groupMembers;
        if (fp) contact.fingerprint = fp;
        if (pk) contact.publicKey = pk;
        state.contacts = [contact, ...state.contacts.filter(c => c.handle.toLowerCase() !== norm)];
    }

    renderCorrespondenceSidebar();
    syncServerSession();
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
        if (headerConnStatusEl) headerConnStatusEl.textContent = 'Connected';
    };

    state.eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data && (data.text || data.isFile)) {
                // Deduplicate incoming stream messages
                if (data.id && seenMessageIDs.has(data.id)) {
                    return;
                }
                if (data.id) seenMessageIDs.add(data.id);

                const isGroup = !!(data.isGroup || (data.group && data.group.startsWith('#')));
                const targetKey = isGroup ? data.group : (data.sender || state.activeTarget);
                const senderName = data.sender || state.activeTarget;
                const timestamp = data.timestamp || formatTime(new Date());
                const msgTTL = data.ttl || getMainTTL(targetKey);

                let isFile = false;
                let fileName = '';
                let fileSize = '';
                let fileType = '';
                let fileData = '';
                let previewText = data.text;

                try {
                    if (typeof data.text === 'string' && (data.text.includes('"__pv_file"') || data.text.includes('"is_file"'))) {
                        const fileObj = JSON.parse(data.text);
                        if (fileObj.__pv_file || fileObj.is_file) {
                            isFile = true;
                            fileName = fileObj.name || fileObj.filename || 'attachment';
                            fileSize = fileObj.size || '';
                            fileType = fileObj.type || fileObj.mime || '';
                            fileData = fileObj.data || fileObj.data_b64 || '';
                            previewText = `📎 ${fileName}`;
                        }
                    }
                } catch (e) {}

                const msg = {
                    id: data.id || ('msg_' + Date.now() + '_' + Math.random().toString(36).substr(2, 5)),
                    sender: senderName,
                    text: previewText,
                    isGroup: isGroup,
                    groupRoom: isGroup ? targetKey : '',
                    isFile: isFile,
                    fileName: fileName,
                    fileSize: fileSize,
                    fileType: fileType,
                    fileData: fileData,
                    timestamp: timestamp,
                    ttl: msgTTL,
                    expiresAt: Date.now() + msgTTL * 1000,
                    isOutgoing: senderName.toLowerCase() === state.myHandle.toLowerCase()
                };

                if (!state.conversations[targetKey]) {
                    state.conversations[targetKey] = [];
                }
                state.conversations[targetKey].push(msg);

                touchContact(targetKey, previewText, timestamp, targetKey, isGroup);

                if (!state.activeTarget) {
                    selectContact(targetKey);
                } else if (targetKey.toLowerCase() === state.activeTarget.toLowerCase()) {
                    appendBubble(msg);
                }
            }
        } catch (err) {
            console.error('Error parsing stream event:', err);
        }
    };

    state.eventSource.onerror = () => {
        state.serverConnected = false;
        if (headerConnStatusEl) headerConnStatusEl.textContent = 'Reconnecting...';
    };
}

// 4. Render Active Conversation Messages
function renderActiveConversation() {
    if (!state.activeTarget || state.contacts.length === 0) {
        chatMessagesContainerEl.innerHTML = `
            <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; padding-top: 120px; text-align:center; color:#64748b;">
                <h3 style="font-size:1.4rem; color:var(--pv-text-main); margin-bottom:8px; font-weight:700;">Pandora's Veil</h3>
                <p style="font-size:0.9rem; max-width:360px; line-height:1.5;">Zero-knowledge cryptographic relay. Start a new chat or join a group to begin.</p>
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

// 5. Append Message Bubble (With Group Sender Identification)
function appendBubble(msg) {
    // Auto-detect JSON file payloads in msg.text if not already flagged
    if (!msg.isFile && typeof msg.text === 'string' && (msg.text.includes('"__pv_file"') || msg.text.includes('"is_file"'))) {
        try {
            const parsed = JSON.parse(msg.text);
            if (parsed.__pv_file || parsed.is_file) {
                msg.isFile = true;
                msg.fileName = parsed.name || parsed.filename || 'attachment';
                msg.fileSize = parsed.size || '';
                msg.fileType = parsed.type || parsed.mime || '';
                msg.fileData = parsed.data || parsed.data_b64 || '';
            }
        } catch (e) {}
    }

    const groupEl = document.createElement('div');
    groupEl.id = msg.id || ('msg_' + Date.now());
    groupEl.className = `pv-bubble-group ${msg.isOutgoing ? 'outgoing' : 'incoming'}`;

    // Sender Tag in Group Chats
    if (state.isGroup && !msg.isOutgoing && msg.sender) {
        const senderTag = document.createElement('div');
        senderTag.className = 'pv-group-sender-tag';
        senderTag.textContent = msg.sender;
        groupEl.appendChild(senderTag);
    }

    if (msg.isFile) {
        const isImage = (msg.fileType && msg.fileType.startsWith('image/')) ||
                        (msg.fileName && msg.fileName.match(/\.(jpeg|jpg|png|gif|webp|svg|bmp)$/i));

        // Format data URI correctly
        let srcUrl = msg.fileData || '';
        if (srcUrl && !srcUrl.startsWith('data:') && !srcUrl.startsWith('blob:') && !srcUrl.startsWith('http')) {
            let mime = msg.fileType;
            if (!mime) {
                if (msg.fileName && msg.fileName.match(/\.png$/i)) mime = 'image/png';
                else if (msg.fileName && msg.fileName.match(/\.gif$/i)) mime = 'image/gif';
                else if (msg.fileName && msg.fileName.match(/\.webp$/i)) mime = 'image/webp';
                else mime = 'image/jpeg';
            }
            srcUrl = `data:${mime};base64,${srcUrl}`;
        }

        const fileCard = document.createElement('div');
        fileCard.className = 'pv-file-card';
        fileCard.style.cursor = srcUrl ? 'pointer' : 'default';

        if (isImage && srcUrl) {
            fileCard.style.flexDirection = 'column';
            fileCard.style.alignItems = 'flex-start';
            fileCard.style.padding = '12px';
            fileCard.style.maxWidth = '300px';

            fileCard.innerHTML = `
                <div style="width:100%; max-height:220px; overflow:hidden; border-radius:10px; margin-bottom:8px; background:#07120e; display:flex; align-items:center; justify-content:center;">
                    <img src="${srcUrl}" alt="${escapeHTML(msg.fileName || 'image')}" onerror="this.parentElement.style.display='none';" style="max-width:100%; max-height:220px; object-fit:contain; border-radius:8px; display:block;">
                </div>
                <div style="display:flex; align-items:center; justify-content:space-between; width:100%; gap:8px;">
                    <div class="pv-file-meta" style="flex:1; overflow:hidden;">
                        <div class="pv-file-name" style="font-size:0.86rem;">${escapeHTML(msg.fileName || 'image.jpeg')}</div>
                        <div class="pv-file-size" style="font-size:0.74rem;">${escapeHTML(msg.fileSize || 'Image file')}</div>
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
                    <div class="pv-file-name">${escapeHTML(msg.fileName || 'file')}</div>
                    <div class="pv-file-size">${escapeHTML(msg.fileSize || 'Attachment')}</div>
                    <div class="pv-file-status">${srcUrl ? '⬇ Click to download' : '✓ Transmitted'}</div>
                </div>
            `;
        }

        if (srcUrl) {
            fileCard.onclick = () => {
                const a = document.createElement('a');
                a.href = srcUrl;
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
    if (chatMessagesScrollEl) {
        chatMessagesScrollEl.scrollTop = chatMessagesScrollEl.scrollHeight;
    }
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
    const effectiveTTL = state.customMsgTTL > 0 ? state.customMsgTTL : getMainTTL(state.activeTarget);
    const msgId = 'msg_' + Date.now() + '_' + Math.random().toString(36).substr(2, 5);

    const msg = {
        id: msgId,
        sender: state.myHandle,
        text: text,
        isGroup: state.isGroup,
        groupRoom: state.isGroup ? state.activeTarget : '',
        timestamp: timestamp,
        ttl: effectiveTTL,
        expiresAt: Date.now() + effectiveTTL * 1000,
        isOutgoing: true
    };

    seenMessageIDs.add(msgId);

    if (!state.conversations[state.activeTarget]) {
        state.conversations[state.activeTarget] = [];
    }
    state.conversations[state.activeTarget].push(msg);
    appendBubble(msg);

    touchContact(state.activeTarget, text, timestamp, state.activeTarget, state.isGroup, state.groupMembers);

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
            fileType: file.type || 'application/octet-stream',
            fileData: base64Data,
            text: `📎 ${file.name}`,
            isGroup: state.isGroup,
            groupRoom: state.isGroup ? state.activeTarget : '',
            timestamp: timestamp,
            ttl: effectiveTTL,
            expiresAt: Date.now() + effectiveTTL * 1000,
            isOutgoing: true
        };

        seenMessageIDs.add(msgId);

        if (!state.conversations[state.activeTarget]) {
            state.conversations[state.activeTarget] = [];
        }
        state.conversations[state.activeTarget].push(msg);
        appendBubble(msg);

        touchContact(state.activeTarget, `📎 ${file.name}`, timestamp, state.activeTarget, state.isGroup, state.groupMembers);

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
            syncServerSession();
        }
    }, 1000);
}

// 8. Setup Event Listeners
function setupEventListeners() {
    if (messageFormEl) {
        messageFormEl.addEventListener('submit', handleSendMessage);
    }
}

// 9. Modals & Actions

// Modal Helpers
function openModal(title, bodyHTML) {
    if (modalTitleEl) modalTitleEl.textContent = title;
    if (modalBodyEl) modalBodyEl.innerHTML = bodyHTML;
    if (modalBackdropEl) modalBackdropEl.classList.remove('hidden');
}

function closeModal() {
    if (modalBackdropEl) modalBackdropEl.classList.add('hidden');
}

function handleBackdropClick(e) {
    if (e.target === modalBackdropEl) {
        closeModal();
    }
}

// Change Handle Modal
function openChangeHandleModal() {
    openModal('Change Device Handle', `
        <form onsubmit="event.preventDefault(); submitChangeHandle();" style="display:flex; flex-direction:column; gap:14px;">
            <p style="font-size:0.86rem; color:#94a3b8; line-height:1.4;">
                Update your identity handle. Your cryptographic keys will be preserved and registered under your new handle on the cloud relay.
            </p>
            <input type="text" id="change-handle-input" value="${escapeHTML(state.myHandle)}" placeholder="New Handle (e.g. Ujwal)" style="width:100%; height:46px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:12px; padding:0 16px; font-size:0.92rem; outline:none;" autofocus>
            <div id="change-handle-error" class="pv-error-box hidden"></div>
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:4px;">
                <button type="button" class="pv-mini-change-btn" style="padding:8px 16px; font-size:0.88rem;" onclick="closeModal()">Cancel</button>
                <button type="submit" class="pv-init-btn" style="height:40px; padding:0 20px; font-size:0.88rem;" id="change-handle-submit-btn">Update Handle</button>
            </div>
        </form>
    `);
    setTimeout(() => {
        const inp = document.getElementById('change-handle-input');
        if (inp) inp.focus();
    }, 100);
}

async function submitChangeHandle() {
    const input = document.getElementById('change-handle-input');
    const errEl = document.getElementById('change-handle-error');
    const btn = document.getElementById('change-handle-submit-btn');

    if (!input || !input.value.trim()) return;
    const newHandle = input.value.trim();

    if (newHandle.toLowerCase() === state.myHandle.toLowerCase()) {
        closeModal();
        return;
    }

    errEl.classList.add('hidden');
    btn.disabled = true;
    btn.textContent = 'Updating...';

    try {
        const token = getAuthToken();
        const res = await fetch('/api/init', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Pandora-Token': token
            },
            body: JSON.stringify({ handle: newHandle })
        });

        const data = await res.json();
        if (res.ok && data.success) {
            state.myHandle = data.handle;
            state.myFingerprint = data.fingerprint;
            state.myPublicKey = data.publicKey;

            if (myHandleLabelEl) myHandleLabelEl.textContent = state.myHandle;
            if (myFingerprintEl) myFingerprintEl.textContent = state.myFingerprint;

            syncServerSession();
            closeModal();
        } else {
            errEl.textContent = data.error || 'Failed to update handle on relay.';
            errEl.classList.remove('hidden');
            btn.disabled = false;
            btn.textContent = 'Update Handle';
        }
    } catch (err) {
        errEl.textContent = `Network error: ${err.message}`;
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Update Handle';
    }
}

// Profile Modal
function toggleProfileModal() {
    openModal('Device Identity (Verified on Relay)', `
        <div style="display:flex; flex-direction:column; gap:12px; font-size:0.88rem;">
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Handle</div>
                <div style="font-family:var(--pv-font-mono); color:#f8fafc; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932; display:flex; justify-content:space-between; align-items:center;">
                    <span>${escapeHTML(state.myHandle)}</span>
                    <button type="button" class="pv-mini-change-btn" onclick="openChangeHandleModal()">Edit</button>
                </div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Device Fingerprint</div>
                <div style="font-family:var(--pv-font-mono); color:#34d399; font-weight:600; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${escapeHTML(state.myFingerprint)}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Public Key (X25519)</div>
                <div style="font-family:var(--pv-font-mono); font-size:0.78rem; color:#94a3b8; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932; word-break:break-all;">${escapeHTML(state.myPublicKey)}</div>
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

    location.reload();
}

// Peer Contact Details Modal
function openContactDetailsModal() {
    if (!state.activeTarget) return;
    const contact = state.contacts.find(c => c.handle.toLowerCase() === state.activeTarget.toLowerCase());
    const displayName = contact ? (contact.displayName || contact.handle) : state.activeTarget;
    const fp = contact ? (contact.fingerprint || state.activeTargetFP || 'Verified') : 'Verified';
    const pk = contact ? (contact.publicKey || state.activeTargetPK || 'Verified on Relay') : 'Verified on Relay';

    if (contact && (contact.isGroup || contact.handle.startsWith('#'))) {
        const memberList = (contact.members || []).map(m => `<li style="padding:4px 0;"><code>${escapeHTML(m)}</code></li>`).join('');
        openModal(`Group: ${displayName}`, `
            <div style="display:flex; flex-direction:column; gap:12px; font-size:0.88rem;">
                <div>
                    <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Group Name</div>
                    <div style="font-family:var(--pv-font-mono); color:#f8fafc; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${escapeHTML(displayName)}</div>
                </div>
                <div>
                    <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Members (${contact.members ? contact.members.length : 0})</div>
                    <ul style="margin:4px 0 0 18px; font-size:0.88rem; color:#94a3b8;">${memberList || '<li>No members recorded</li>'}</ul>
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
                <div style="font-family:var(--pv-font-mono); color:#f8fafc; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${escapeHTML(state.activeTarget)}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Device Fingerprint</div>
                <div style="font-family:var(--pv-font-mono); color:#34d399; font-weight:600; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932;">${escapeHTML(fp)}</div>
            </div>
            <div>
                <div style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; margin-bottom:4px;">Public Key</div>
                <div style="font-family:var(--pv-font-mono); font-size:0.78rem; color:#94a3b8; background:#141a20; padding:8px 12px; border-radius:8px; border:1px solid #202932; word-break:break-all;">${escapeHTML(pk)}</div>
            </div>
        </div>
        <div style="display:flex; justify-content:flex-end; margin-top:20px;">
            <button type="button" style="background:#ef4444; color:#fff; border:none; padding:8px 16px; border-radius:8px; font-weight:600; cursor:pointer;" onclick="removeCorrespondence('${state.activeTarget}')">Remove Correspondence</button>
        </div>
    `);
}

function removeCorrespondence(handle) {
    const norm = handle.toLowerCase();
    state.contacts = state.contacts.filter(c => c.handle.toLowerCase() !== norm);
    delete state.conversations[handle];
    delete state.convTTL[handle];

    if (state.activeTarget.toLowerCase() === norm) {
        state.activeTarget = state.contacts.length > 0 ? state.contacts[0].handle : '';
    }

    syncServerSession();
    closeModal();
    renderCorrespondenceSidebar();
    renderActiveConversation();
}

// Add New Chat Modal
function openNewChatModal() {
    openModal('New Chat', `
        <form onsubmit="event.preventDefault(); startNewPeerFromModal();" style="display:flex; flex-direction:column; gap:14px;">
            <input type="text" id="new-peer-handle-input" placeholder="Peer handle (e.g. Ujwal, Bob, Alice)" style="width:100%; height:46px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:12px; padding:0 16px; font-size:0.92rem; outline:none;" autofocus>
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

async function startNewPeerFromModal() {
    const input = document.getElementById('new-peer-handle-input');
    const errEl = document.getElementById('new-peer-error');
    const btn = document.getElementById('add-peer-submit-btn');

    if (!input || !input.value.trim()) return;
    const handle = input.value.trim();

    if (handle.toLowerCase() === state.myHandle.toLowerCase()) {
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
            touchContact(peerHandle, 'Connected', formatTime(new Date()), peerHandle, false, [], data.fingerprint, data.publicKey);
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

// Join Group Modal
function openJoinGroupModal() {
    openModal('Join Group Room', `
        <form onsubmit="event.preventDefault(); submitJoinGroup();" style="display:flex; flex-direction:column; gap:14px;">
            <p style="font-size:0.86rem; color:#94a3b8; line-height:1.4;">
                Enter the name of an encrypted group room.
            </p>
            <input type="text" id="join-group-name-input" placeholder="Group Room (e.g. #Security-Team)" style="width:100%; height:46px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:12px; padding:0 16px; font-size:0.92rem; outline:none;" autofocus>
            <div id="join-group-error" class="pv-error-box hidden"></div>
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:4px;">
                <button type="button" class="pv-mini-change-btn" style="padding:8px 16px; font-size:0.88rem;" onclick="closeModal()">Cancel</button>
                <button type="submit" class="pv-init-btn" style="height:40px; padding:0 22px; font-size:0.88rem;" id="join-group-submit-btn">Join Room</button>
            </div>
        </form>
    `);
    setTimeout(() => {
        const inp = document.getElementById('join-group-name-input');
        if (inp) inp.focus();
    }, 100);
}

function submitJoinGroup() {
    const input = document.getElementById('join-group-name-input');
    const errEl = document.getElementById('join-group-error');

    if (!input || !input.value.trim()) return;
    let groupName = input.value.trim();
    if (!groupName.startsWith('#')) groupName = '#' + groupName;

    touchContact(groupName, 'Joined group room', formatTime(new Date()), groupName, true, []);
    selectContact(groupName);
    closeModal();
}

// Interactive New Group Modal with Contact Multi-select & Search Lookup
let selectedGroupMembers = [];

function openNewGroupModal() {
    selectedGroupMembers = [];
    const directContacts = state.contacts.filter(c => !c.isGroup && !c.handle.startsWith('#'));

    let contactsHTML = '';
    if (directContacts.length > 0) {
        contactsHTML = directContacts.map(c => `
            <label class="pv-contact-checkbox-item">
                <input type="checkbox" value="${escapeHTML(c.handle)}" onchange="toggleGroupMemberSelection('${escapeHTML(c.handle)}', this.checked)">
                <span>${escapeHTML(c.displayName || c.handle)}</span>
            </label>
        `).join('');
    } else {
        contactsHTML = '<div style="font-size:0.8rem; color:#64748b; padding:4px;">No direct contacts yet</div>';
    }

    openModal('Create New Group', `
        <form onsubmit="event.preventDefault(); submitCreateGroupModal();" style="display:flex; flex-direction:column; gap:12px;">
            <div>
                <label style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; display:block; margin-bottom:4px;">Group Name</label>
                <input type="text" id="new-group-name-input" placeholder="e.g. #Alpha_Team" style="width:100%; height:44px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:10px; padding:0 14px; font-size:0.9rem; outline:none;" autofocus>
            </div>

            <div>
                <label style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; display:block; margin-bottom:4px;">Selected Members</label>
                <div class="pv-chips-container" id="group-member-chips">
                    <span style="font-size:0.8rem; color:#64748b;">(No members added yet)</span>
                </div>
            </div>

            <div>
                <label style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; display:block; margin-bottom:4px;">Select from Contacts</label>
                <div class="pv-contacts-picker-box">
                    ${contactsHTML}
                </div>
            </div>

            <div>
                <label style="font-size:0.75rem; color:#64748b; text-transform:uppercase; font-weight:700; display:block; margin-bottom:4px;">Search & Add New Member</label>
                <div style="display:flex; gap:8px;">
                    <input type="text" id="group-member-search-input" placeholder="Enter username to search on server..." style="flex:1; height:38px; background:#11161c; border:1px solid #1e2630; color:#f8fafc; border-radius:8px; padding:0 12px; font-size:0.86rem; outline:none;" onkeydown="if(event.key==='Enter'){event.preventDefault(); searchGroupMember();}">
                    <button type="button" class="pv-mini-change-btn" style="padding:0 14px; font-size:0.82rem;" onclick="searchGroupMember()">Search</button>
                </div>
                <div id="group-member-search-result"></div>
            </div>

            <div id="new-group-error" class="pv-error-box hidden"></div>
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:4px;">
                <button type="button" class="pv-mini-change-btn" style="padding:8px 16px; font-size:0.88rem;" onclick="closeModal()">Cancel</button>
                <button type="submit" class="pv-init-btn" style="height:40px; padding:0 22px; font-size:0.88rem;" id="add-group-submit-btn">Create Group</button>
            </div>
        </form>
    `);

    setTimeout(() => {
        const inp = document.getElementById('new-group-name-input');
        if (inp) inp.focus();
    }, 100);
}

function updateGroupMemberChips() {
    const container = document.getElementById('group-member-chips');
    if (!container) return;

    if (selectedGroupMembers.length === 0) {
        container.innerHTML = '<span style="font-size:0.8rem; color:#64748b;">(No members added yet)</span>';
        return;
    }

    container.innerHTML = selectedGroupMembers.map(m => `
        <span class="pv-member-chip">
            <span>${escapeHTML(m)}</span>
            <span class="pv-chip-remove" onclick="removeGroupMemberChip('${escapeHTML(m)}')">×</span>
        </span>
    `).join('');
}

function toggleGroupMemberSelection(handle, checked) {
    if (checked) {
        if (!selectedGroupMembers.some(m => m.toLowerCase() === handle.toLowerCase())) {
            selectedGroupMembers.push(handle);
        }
    } else {
        selectedGroupMembers = selectedGroupMembers.filter(m => m.toLowerCase() !== handle.toLowerCase());
    }
    updateGroupMemberChips();
}

function removeGroupMemberChip(handle) {
    selectedGroupMembers = selectedGroupMembers.filter(m => m.toLowerCase() !== handle.toLowerCase());
    updateGroupMemberChips();
    const chk = document.querySelector(`.pv-contacts-picker-box input[value="${handle}"]`);
    if (chk) chk.checked = false;
}

async function searchGroupMember() {
    const input = document.getElementById('group-member-search-input');
    const resultBox = document.getElementById('group-member-search-result');
    if (!input || !input.value.trim() || !resultBox) return;

    const query = input.value.trim();
    if (query.toLowerCase() === state.myHandle.toLowerCase()) {
        resultBox.innerHTML = '<div style="font-size:0.8rem; color:#f59e0b; margin-top:6px;">You are already in the group as creator.</div>';
        return;
    }

    resultBox.innerHTML = '<div style="font-size:0.8rem; color:#64748b; margin-top:6px;">Searching server...</div>';

    try {
        const token = getAuthToken();
        const res = await fetch(`/api/lookup?handle=${encodeURIComponent(query)}`, {
            headers: { 'X-Pandora-Token': token }
        });
        const data = await res.json();

        if (res.ok && data.publicKey) {
            const foundHandle = data.handle || query;
            resultBox.innerHTML = `
                <div class="pv-search-result-card">
                    <div class="pv-search-result-info">
                        <div class="pv-search-result-name">${escapeHTML(foundHandle)}</div>
                        <div class="pv-search-result-fp">FP: ${escapeHTML(data.fingerprint || 'Verified')}</div>
                    </div>
                    <button type="button" class="pv-add-member-btn" onclick="addFoundMemberToGroup('${escapeHTML(foundHandle)}')">+ Add</button>
                </div>
            `;
        } else {
            resultBox.innerHTML = `<div style="font-size:0.8rem; color:#ef4444; margin-top:6px;">${escapeHTML(data.error || `User '${query}' not found on server.`)}</div>`;
        }
    } catch (e) {
        resultBox.innerHTML = `<div style="font-size:0.8rem; color:#ef4444; margin-top:6px;">Lookup error: ${escapeHTML(e.message)}</div>`;
    }
}

function addFoundMemberToGroup(handle) {
    if (!selectedGroupMembers.some(m => m.toLowerCase() === handle.toLowerCase())) {
        selectedGroupMembers.push(handle);
        updateGroupMemberChips();
    }
    const resultBox = document.getElementById('group-member-search-result');
    if (resultBox) resultBox.innerHTML = `<div style="font-size:0.8rem; color:#10b981; margin-top:6px;">Added ${escapeHTML(handle)} to group.</div>`;
}

function submitCreateGroupModal() {
    const nameInput = document.getElementById('new-group-name-input');
    const errEl = document.getElementById('new-group-error');

    if (!nameInput || !nameInput.value.trim()) {
        errEl.textContent = 'Please enter a group name.';
        errEl.classList.remove('hidden');
        return;
    }

    let groupName = nameInput.value.trim();
    if (!groupName.startsWith('#')) groupName = '#' + groupName;

    touchContact(groupName, 'Group Created', formatTime(new Date()), groupName, true, selectedGroupMembers);
    selectContact(groupName);
    closeModal();
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
        state.customMsgTTL = val;
        if (topMainTtlLabelEl) {
            topMainTtlLabelEl.textContent = `${formatTTL(val)} Lifespan`;
        }
        updateMsgTTLBadge();
        syncServerSession();
    }
    closeModal();
}

// 10. Lifecycle Boot
document.addEventListener('DOMContentLoaded', () => {
    initApp();
    if (initOverlayEl) {
        initOverlayEl.addEventListener('submit', handleInitSubmit);
    }
});
