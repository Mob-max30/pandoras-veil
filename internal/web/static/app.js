// ==========================================================================
// PANDORA'S VEIL | Linen Aesthetic Logic & Persistent Connection History
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
    const clean = handle.replace(/^PV-/, '').replace(/^#/, '');
    const parts = clean.split(/[\s_-]+/);
    if (parts.length >= 2) {
        return (parts[0][0] + parts[1][0]).toUpperCase();
    }
    return clean.slice(0, 2).toUpperCase() || 'P';
}

const DEFAULT_CONTACTS = [
    {
        handle: 'PV-PRANAV',
        name: 'Pranav',
        fp: '1E42-2834',
        type: 'dm',
        lastMessage: 'Live end-to-end encrypted session active.',
        time: '21:10'
    },
    {
        handle: 'PV-PAVAN',
        name: 'Pavan',
        fp: '7C11-884A',
        type: 'dm',
        lastMessage: 'Redis TTL offline queueing verified.',
        time: '20:55'
    },
    {
        handle: 'Development',
        name: 'Development',
        fp: 'Group',
        type: 'group',
        members: ['PV-PRANAV', 'PV-PAVAN', 'PV-ALICE', 'PV-BOB'],
        lastMessage: 'Alice: Deployment verified on Render.',
        time: 'Yesterday'
    },
    {
        handle: 'PV-BOB',
        name: 'Bob',
        fp: '915E-B66D',
        type: 'dm',
        lastMessage: 'Key verified: 915E-B66D',
        time: 'Yesterday'
    },
    {
        handle: 'PV-ALICE',
        name: 'Alice',
        fp: '47B7-9F60',
        type: 'dm',
        lastMessage: 'Device key registered.',
        time: '22/08'
    }
];

const DEFAULT_CONVERSATIONS = {
    'PV-PRANAV': [
        {
            sender: 'PV-PRANAV',
            text: 'Live end-to-end encrypted session active on Pandora\'s Veil.',
            timestamp: '21:10',
            isOutgoing: false
        }
    ],
    'PV-PAVAN': [
        {
            sender: 'PV-PAVAN',
            text: 'Redis TTL offline queueing verified on cloud relay.',
            timestamp: '20:55',
            isOutgoing: false
        }
    ],
    'Development': [
        {
            sender: 'PV-ALICE',
            text: 'Deployment verified on Render relay.',
            timestamp: 'Yesterday',
            isOutgoing: false
        }
    ],
    'PV-BOB': [
        {
            sender: 'PV-BOB',
            text: 'Connected. Device fingerprint 915E-B66D confirmed.',
            timestamp: 'Yesterday',
            isOutgoing: false
        }
    ],
    'PV-ALICE': [
        {
            sender: 'PV-ALICE',
            text: 'Device key registered.',
            timestamp: '22/08',
            isOutgoing: false
        }
    ]
};

const state = {
    myHandle: 'PV-UJWAL',
    myFingerprint: 'BA64-5843',
    myPublicKey: 'age1q8ulqk4630rwqwavdst4fegn9st2zmrqdczvrx4uec9cmu6ah55swres5x',
    activeTarget: 'PV-PRANAV',
    isGroup: false,
    groupMembers: ['PV-PRANAV', 'PV-PAVAN', 'PV-ALICE', 'PV-BOB'],
    ttl: 300,
    burnAfterReading: true,
    eventSource: null,
    contacts: [],
    conversations: {}
};

function getAuthToken() {
    const meta = document.querySelector('meta[name="pandora-token"]');
    return meta ? meta.getAttribute('content') : '';
}

// Persistence Utilities
function loadPersistedData() {
    try {
        const savedContacts = localStorage.getItem('pandora_contacts_v2');
        const savedConvs = localStorage.getItem('pandora_conversations_v2');
        const savedTarget = localStorage.getItem('pandora_active_target_v2');

        if (savedContacts) {
            state.contacts = JSON.parse(savedContacts);
        } else {
            state.contacts = DEFAULT_CONTACTS;
        }

        if (savedConvs) {
            state.conversations = JSON.parse(savedConvs);
        } else {
            state.conversations = DEFAULT_CONVERSATIONS;
        }

        if (savedTarget && state.contacts.some(c => c.handle === savedTarget)) {
            state.activeTarget = savedTarget;
        } else if (state.contacts.length > 0) {
            state.activeTarget = state.contacts[0].handle;
        }
    } catch (e) {
        state.contacts = DEFAULT_CONTACTS;
        state.conversations = DEFAULT_CONVERSATIONS;
    }
}

function savePersistedData() {
    try {
        localStorage.setItem('pandora_contacts_v2', JSON.stringify(state.contacts));
        localStorage.setItem('pandora_conversations_v2', JSON.stringify(state.conversations));
        localStorage.setItem('pandora_active_target_v2', state.activeTarget);
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
const chatMessagesContainerEl = document.getElementById('chat-messages-container');
const chatMessagesScrollEl = document.getElementById('chat-messages-scroll');
const chatInputEl = document.getElementById('chat-input');
const messageFormEl = document.getElementById('message-form');
const modalBackdropEl = document.getElementById('modal-backdrop');
const modalTitleEl = document.getElementById('modal-title');
const modalBodyEl = document.getElementById('modal-body');
const chatSearchEl = document.getElementById('chat-search');

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
            state.myHandle = data.handle || state.myHandle;
            state.myFingerprint = data.fingerprint || state.myFingerprint;
            state.myPublicKey = data.publicKey || state.myPublicKey;

            myHandleEl.textContent = state.myHandle;
            myFingerprintEl.textContent = `FP: ${state.myFingerprint}`;
            myAvatarEl.textContent = getInitials(state.myHandle).charAt(0) || 'U';
        }
    } catch (err) {
        console.warn('Failed to load local identity:', err);
    }

    renderCorrespondenceSidebar();
    setupEventListeners();
    connectSSEStream();
    renderActiveConversation();
}

// 2. Render Correspondence List in Sidebar
function renderCorrespondenceSidebar() {
    chatsListEl.innerHTML = '';

    state.contacts.forEach(contact => {
        const initials = getInitials(contact.name || contact.handle);
        const colorClass = getAvatarColor(contact.handle);
        const isActive = contact.handle === state.activeTarget;

        const card = document.createElement('div');
        card.className = `chat-card ${isActive ? 'active' : ''}`;
        card.setAttribute('data-handle', contact.handle);
        card.setAttribute('data-name', contact.name || contact.handle);
        card.setAttribute('data-fp', contact.fp || 'Verified');
        card.setAttribute('data-type', contact.type || 'dm');

        card.innerHTML = `
            <div class="avatar ${colorClass}">${initials}</div>
            <div class="chat-card-content">
                <div class="chat-card-header">
                    <span class="contact-card-name">${contact.name || contact.handle}</span>
                    <span class="card-time" id="time-${contact.handle}">${contact.time || ''}</span>
                </div>
                <div class="chat-card-footer">
                    <span class="card-snippet" id="preview-${contact.handle}">${contact.lastMessage || 'Connected'}</span>
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
        activeFpSubtleEl.textContent = `• ${activeContact.fp || 'Verified'}`;
        state.isGroup = activeContact.type === 'group';
    }
}

function selectContact(handle) {
    state.activeTarget = handle;
    savePersistedData();
    renderCorrespondenceSidebar();
    renderActiveConversation();
    chatInputEl.focus();
}

function touchContact(handle, lastMsgText, timestamp, senderName) {
    let contact = state.contacts.find(c => c.handle === handle);
    if (!contact) {
        contact = {
            handle: handle,
            name: senderName || handle,
            fp: 'Verified',
            type: handle.includes(',') ? 'group' : 'dm',
            lastMessage: lastMsgText,
            time: timestamp
        };
        state.contacts.unshift(contact);
    } else {
        contact.lastMessage = lastMsgText;
        contact.time = timestamp;
        // Move to top of list
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

    state.eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data && data.text) {
                const senderName = data.sender || state.activeTarget;
                const timestamp = data.timestamp || formatTime(new Date());
                const msg = {
                    sender: senderName,
                    text: data.text,
                    timestamp: timestamp,
                    isOutgoing: false
                };

                if (!state.conversations[senderName]) {
                    state.conversations[senderName] = [];
                }
                state.conversations[senderName].push(msg);

                touchContact(senderName, data.text, timestamp, senderName);

                if (senderName === state.activeTarget || (state.isGroup && senderName !== state.myHandle)) {
                    appendLinenBubble(msg);
                }
            }
        } catch (err) {
            console.error('Error parsing stream event:', err);
        }
    };
}

// 4. Render Active Conversation Messages
function renderActiveConversation() {
    const separatorHTML = `
        <div class="linen-date-separator">
            <span class="sep-line"></span>
            <span class="sep-label">TODAY</span>
            <span class="sep-line"></span>
        </div>
    `;

    chatMessagesContainerEl.innerHTML = separatorHTML;

    const msgs = state.conversations[state.activeTarget] || [];
    msgs.forEach(msg => appendLinenBubble(msg));

    scrollChatToBottom();
}

// 5. Append Linen Message Bubble
function appendLinenBubble(msg) {
    const groupEl = document.createElement('div');
    groupEl.className = `linen-bubble-group ${msg.isOutgoing ? 'outgoing-group' : 'incoming-group'}`;

    const cardEl = document.createElement('div');
    cardEl.className = `linen-card ${msg.isOutgoing ? 'outgoing-card' : 'incoming-card'}`;

    const pEl = document.createElement('p');
    pEl.textContent = msg.text;
    cardEl.appendChild(pEl);
    groupEl.appendChild(cardEl);

    if (!msg.isOutgoing && msg.timestamp) {
        const timeSpan = document.createElement('span');
        timeSpan.className = 'bubble-time-stamp';
        timeSpan.textContent = msg.timestamp;
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

    chatInputEl.value = '';

    const timestamp = formatTime(new Date());
    const msg = {
        sender: state.myHandle,
        text: text,
        timestamp: timestamp,
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
            ttl: state.ttl,
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
                sender: 'SYSTEM',
                text: `Delivery failed: ${errData.error || 'Relay error'}`,
                timestamp: formatTime(new Date()),
                isOutgoing: false
            });
        }
    } catch (err) {
        appendLinenBubble({
            sender: 'SYSTEM',
            text: `Network error: ${err.message}`,
            timestamp: formatTime(new Date()),
            isOutgoing: false
        });
    }
}

// 7. Setup Event Listeners
function setupEventListeners() {
    messageFormEl.addEventListener('submit', handleSendMessage);

    // Chat Search Filter
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

// 8. Modals
function toggleProfileModal() {
    openModal('Device Security Credentials', `
        <div style="display:flex; flex-direction:column; gap:16px; font-size:0.92rem; color:var(--linen-text-dark);">
            <div><strong style="color:#636765;">Handle:</strong> <span style="font-weight:600;">${state.myHandle}</span></div>
            <div><strong style="color:#636765;">Fingerprint:</strong> <span style="color:var(--linen-card-green); font-family:var(--font-mono); font-weight:600;">${state.myFingerprint}</span></div>
            <div><strong style="color:#636765;">Public Key:</strong> <div style="font-size:0.75rem; color:#787b78; word-break:break-all; font-family:var(--font-mono); margin-top:6px; background:#fff; padding:10px; border-radius:8px;">${state.myPublicKey}</div></div>
            <div style="color:var(--linen-card-green); font-size:0.82rem; margin-top:4px;">Keypair secured on physical device.</div>
        </div>
    `);
}

function openContactDetailsModal() {
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;
    const fp = contact ? (contact.fp || 'Verified') : 'Verified';

    openModal(`${displayName}`, `
        <div style="display:flex; flex-direction:column; gap:16px; font-size:0.92rem; color:var(--linen-text-dark);">
            <div><strong style="color:#636765;">Handle:</strong> <span>${state.activeTarget}</span></div>
            <div><strong style="color:#636765;">Device Fingerprint:</strong> <span style="font-family:var(--font-mono); color:var(--linen-card-green); font-weight:600;">${fp}</span></div>
            <div><strong style="color:#636765;">Encryption:</strong> <span>age / X25519 Native Go Bridge</span></div>
            <div><strong style="color:#636765;">Status:</strong> <span style="color:var(--linen-online); font-weight:600;">Connected</span></div>
        </div>
    `);
}

function openNewChatModal() {
    openModal('Start Correspondence', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Enter recipient handle or comma-separated handles for a group:</p>
            <input type="text" id="new-chat-handle-input" placeholder="e.g. PV-BOB or PV-BOB,PV-ALICE" style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-size:0.95rem; outline:none;">
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:8px;">
                <button type="button" class="linen-btn" onclick="startNewChatFromModal()">Start Correspondence</button>
            </div>
        </div>
    `);
}

function startNewChatFromModal() {
    const input = document.getElementById('new-chat-handle-input');
    if (input && input.value.trim()) {
        const val = input.value.trim();
        touchContact(val, 'Connected', formatTime(new Date()), val);
        selectContact(val);
        closeModal();
    }
}

function openSecretDepositModal() {
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;

    openModal('Send Confidential Deposit (GETDEL)', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Enter confidential secret (API keys, recovery codes) to deposit for <strong>${displayName}</strong>:</p>
            <textarea id="secret-deposit-textarea" rows="4" placeholder="Confidential payload..." style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-family:var(--font-mono); font-size:0.88rem; outline:none;"></textarea>
            <div style="display:flex; justify-content:space-between; align-items:center; margin-top:8px;">
                <span style="font-size:0.8rem; color:var(--linen-terracotta);">Burn-After-Reading: ON (${state.ttl}s TTL)</span>
                <button type="button" class="linen-btn" onclick="submitSecretDepositFromModal()">Deposit Secret</button>
            </div>
        </div>
    `);
}

async function submitSecretDepositFromModal() {
    const textarea = document.getElementById('secret-deposit-textarea');
    if (textarea && textarea.value.trim()) {
        const secret = textarea.value.trim();
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
                    ttl: state.ttl
                })
            });

            if (res.ok) {
                const data = await res.json();
                appendLinenBubble({
                    sender: state.myHandle,
                    text: `[Self-Destructing Deposit Created] ID: ${data.id} (Destroyed upon first read by recipient)`,
                    timestamp: formatTime(new Date()),
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

function openDisappearingModal() {
    const contact = state.contacts.find(c => c.handle === state.activeTarget);
    const displayName = contact ? (contact.name || contact.handle) : state.activeTarget;

    openModal('Disappearing Messages Timer', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Set message lifespan timer for messages sent to ${displayName}:</p>
            <div style="display:flex; flex-direction:column; gap:10px;">
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="60" ${state.ttl === 60 ? 'checked' : ''}> 60 Seconds</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="300" ${state.ttl === 300 ? 'checked' : ''}> 5 Minutes (300s)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="3600" ${state.ttl === 3600 ? 'checked' : ''}> 1 Hour</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="86400" ${state.ttl === 86400 ? 'checked' : ''}> 24 Hours</label>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="button" class="linen-btn" onclick="applyTtlFromModal()">Apply Timer</button>
            </div>
        </div>
    `);
}

function applyTtlFromModal() {
    const selected = document.querySelector('input[name="ttl-opt"]:checked');
    if (selected) {
        state.ttl = parseInt(selected.value, 10) || 300;
        closeModal();
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

// Start
document.addEventListener('DOMContentLoaded', initApp);
