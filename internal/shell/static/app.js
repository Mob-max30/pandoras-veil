document.addEventListener('DOMContentLoaded', () => {
  const cmdInput = document.getElementById('cmd-input');
  const fileBtn = document.getElementById('file-picker-btn');
  const msgContainer = document.getElementById('message-container');
  const headerChannelTitle = document.getElementById('header-channel-title');
  const cmdChannelTag = document.getElementById('cmd-channel-tag');
  const recipMeta = document.getElementById('recip-meta');
  const burnToggle = document.getElementById('burn-toggle');
  const burnText = document.getElementById('burn-text');
  const ttlBtns = document.querySelectorAll('.ttl-btn');
  const dmList = document.getElementById('dm-list');
  const groupList = document.getElementById('group-list');
  const addDmBtn = document.getElementById('add-dm-btn');
  const addGroupBtn = document.getElementById('add-group-btn');

  let activeChannel = '#Development';
  let activeTTL = 300;
  let isBurnActive = true;

  // TTL Selection
  ttlBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      ttlBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      activeTTL = parseInt(btn.dataset.ttl, 10);
    });
  });

  // Burn Toggle
  burnToggle.addEventListener('change', () => {
    isBurnActive = burnToggle.checked;
    burnText.textContent = isBurnActive ? '*ON*' : 'OFF';
    burnText.className = isBurnActive ? 'green' : 'dim';
  });

  function bindChannelEvents() {
    document.querySelectorAll('.channel-item').forEach(item => {
      item.onclick = () => {
        document.querySelectorAll('.channel-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        activeChannel = item.dataset.handle || item.dataset.group;
        headerChannelTitle.textContent = activeChannel;
        cmdChannelTag.textContent = activeChannel;
        recipMeta.textContent = activeChannel + ' ..';
      };
    });
  }

  bindChannelEvents();

  // Add DM
  addDmBtn.addEventListener('click', () => {
    const handle = prompt('Enter recipient handle (e.g., PV-UJWAL):');
    if (!handle) return;
    const cleanHandle = handle.trim().toUpperCase();
    if (!cleanHandle) return;

    const emptyHint = dmList.querySelector('.empty-hint');
    if (emptyHint) emptyHint.remove();

    let existing = dmList.querySelector(`[data-handle="${cleanHandle}"]`);
    if (!existing) {
      existing = document.createElement('li');
      existing.className = 'channel-item';
      existing.dataset.handle = cleanHandle;
      existing.innerHTML = `<span class="dot online"></span> ${cleanHandle}`;
      dmList.appendChild(existing);
      bindChannelEvents();
    }
    existing.click();
  });

  // Add Group
  addGroupBtn.addEventListener('click', () => {
    const name = prompt('Enter group chat name (e.g., #Security_Team):');
    if (!name) return;
    let cleanGroup = name.trim();
    if (!cleanGroup.startsWith('#')) cleanGroup = '#' + cleanGroup;

    let existing = groupList.querySelector(`[data-group="${cleanGroup}"]`);
    if (!existing) {
      existing = document.createElement('li');
      existing.className = 'channel-item';
      existing.dataset.group = cleanGroup;
      existing.innerHTML = `<span class="hash">#</span>${cleanGroup.replace('#', '')} <span class="dot online"></span>`;
      groupList.appendChild(existing);
      bindChannelEvents();
    }
    existing.click();
  });

  function scrollToBottom() {
    setTimeout(() => {
      msgContainer.scrollTop = msgContainer.scrollHeight + 10000;
    }, 50);
  }

  scrollToBottom();

  // File Picker Trigger (/f)
  fileBtn.addEventListener('click', () => {
    triggerFileAttachment();
  });

  function triggerFileAttachment() {
    const input = document.createElement('input');
    input.type = 'file';
    input.onchange = e => {
      const file = e.target.files[0];
      if (!file) return;

      const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      const row = document.createElement('div');
      row.className = 'chat-bubble-row right';
      row.innerHTML = `
        <div class="bubble-meta">[${timeStr}] <span class="you-badge">[YOU]</span></div>
        <div class="chat-bubble right-bubble">
          📁 [FILE SENT] ${file.name} (${Math.round(file.size / 1024)} KB)
        </div>
      `;
      msgContainer.appendChild(row);
      scrollToBottom();
    };
    input.click();
  }

  // Sending Messages
  cmdInput.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      const text = cmdInput.value.trim();
      if (!text) return;

      if (text === '/f' || text === '/file' || text === '/attach') {
        triggerFileAttachment();
        cmdInput.value = '';
        return;
      }

      const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      const row = document.createElement('div');
      row.className = 'chat-bubble-row right';
      row.innerHTML = `
        <div class="bubble-meta">[${timeStr}] <span class="you-badge">[YOU]</span></div>
        <div class="chat-bubble right-bubble">
          ${escapeHTML(text)}
        </div>
      `;
      msgContainer.appendChild(row);
      scrollToBottom();

      cmdInput.value = '';
    }
  });

  function escapeHTML(str) {
    return str.replace(/[&<>'"]/g, 
      tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
    );
  }
});
