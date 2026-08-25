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

  let activeChannel = 'PV-UJWAL';
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

  // Channel Item Selection
  document.querySelectorAll('.channel-item').forEach(item => {
    item.addEventListener('click', () => {
      document.querySelectorAll('.channel-item').forEach(i => i.classList.remove('active'));
      item.classList.add('active');
      activeChannel = item.dataset.handle || item.dataset.group;
      headerChannelTitle.textContent = activeChannel;
      cmdChannelTag.textContent = activeChannel;
      recipMeta.textContent = activeChannel + ' ..';
    });
  });

  function scrollToBottom() {
    requestAnimationFrame(() => {
      msgContainer.scrollTop = msgContainer.scrollHeight;
    });
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
