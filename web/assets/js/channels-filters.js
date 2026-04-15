// Filter channels based on current filters
let filteredChannels = []; // 存储筛选后的渠道列表
let modelFilterOptions = [];
let modelFilterCombobox = null; // 通用组件实例
let channelNameCombobox = null; // 渠道名筛选组合框实例

function getModelAllLabel() {
  return (window.t && window.t('channels.modelAll')) || '所有模型';
}

function getChannelNameAllLabel() {
  return (window.t && window.t('channels.channelNameAll')) || '所有渠道';
}

function modelFilterInputValueFromFilterValue(filterValue) {
  if (!filterValue || filterValue === 'all') return getModelAllLabel();
  return filterValue;
}

function normalizeModelFilterOption() {
  if (!filters || !filters.model || filters.model === 'all') return false;
  if (modelFilterOptions.includes(filters.model)) return false;

  filters.model = 'all';
  if (typeof saveChannelsFilters === 'function') saveChannelsFilters();
  return true;
}

function filterChannels() {
  const filtered = channels.filter(channel => {
    if (filters.search && channel.name !== filters.search) {
      return false;
    }

    if (filters.channelType !== 'all') {
      const channelType = channel.channel_type || 'anthropic';
      if (channelType !== filters.channelType) {
        return false;
      }
    }

    if (filters.status !== 'all') {
      if (filters.status === 'enabled' && !channel.enabled) return false;
      if (filters.status === 'disabled' && channel.enabled) return false;
      if (filters.status === 'cooldown' && !(channel.cooldown_remaining_ms > 0)) return false;
    }

    if (filters.model !== 'all') {
      // 新格式：models 是 {model, redirect_model} 对象数组
      const modelNames = Array.isArray(channel.models)
        ? channel.models.map(m => m.model || m)
        : [];
      if (!modelNames.includes(filters.model)) {
        return false;
      }
    }

    return true;
  });

  // 排序：优先使用 effective_priority（健康度模式），否则使用 priority
  filtered.sort((a, b) => {
    const prioA = a.effective_priority ?? a.priority;
    const prioB = b.effective_priority ?? b.priority;
    if (prioB !== prioA) {
      return prioB - prioA;
    }
    const typeA = (a.channel_type || 'anthropic').toLowerCase();
    const typeB = (b.channel_type || 'anthropic').toLowerCase();
    if (typeA !== typeB) {
      return typeA.localeCompare(typeB);
    }
    return a.name.localeCompare(b.name);
  });

  filteredChannels = filtered; // 保存筛选后的列表供其他模块使用
  renderChannels(filtered);
  updateFilterInfo(filtered.length, channels.length);

  // 更新解除冷却按钮红点
  if (typeof updateClearCooldownBadge === 'function') {
    updateClearCooldownBadge();
  }
}

// Update filter info display
function updateFilterInfo(filtered, total) {
  document.getElementById('filteredCount').textContent = filtered;
  document.getElementById('totalCount').textContent = total;
}

// Update model filter options
function updateModelOptions() {
  const modelSet = new Set();
  const typeFilter = (filters && filters.channelType) ? filters.channelType : 'all';
  channels.forEach(channel => {
    if (typeFilter !== 'all') {
      const channelType = channel.channel_type || 'anthropic';
      if (channelType !== typeFilter) return;
    }
    if (Array.isArray(channel.models)) {
      // 新格式：models 是 {model, redirect_model} 对象数组
      channel.models.forEach(m => {
        const modelName = m.model || m;
        if (modelName) modelSet.add(modelName);
      });
    }
  });

  modelFilterOptions = Array.from(modelSet).sort();

  normalizeModelFilterOption();

  // 使用通用组件刷新下拉框
  if (modelFilterCombobox) {
    modelFilterCombobox.setValue(filters.model, modelFilterInputValueFromFilterValue(filters.model));
    modelFilterCombobox.refresh();
  } else {
    const modelFilterInput = document.getElementById('modelFilter');
    if (modelFilterInput) {
      modelFilterInput.value = modelFilterInputValueFromFilterValue(filters.model);
    }
  }
}

// 更新渠道名称下拉选项（getOptions 回调动态读取，refresh 触发重算）
function updateChannelNameOptions() {
  if (!channelNameCombobox) return;

  // 检查当前选值是否仍合法
  const currentVal = channelNameCombobox.getValue();
  if (currentVal) {
    const typeFilter = (filters && filters.channelType) ? filters.channelType : 'all';
    const stillExists = channels.some(ch => {
      if (typeFilter !== 'all' && (ch.channel_type || 'anthropic') !== typeFilter) return false;
      return ch.name === currentVal;
    });
    if (!stillExists) {
      channelNameCombobox.setValue('', getChannelNameAllLabel());
      filters.search = '';
      if (typeof saveChannelsFilters === 'function') saveChannelsFilters();
    }
  }

  channelNameCombobox.refresh();
}

// Setup filter event listeners
function setupFilterListeners() {
  document.getElementById('statusFilter').addEventListener('change', (e) => {
    filters.status = e.target.value;
    if (typeof saveChannelsFilters === 'function') saveChannelsFilters();
    filterChannels();
  });

  // 模型筛选 combobox
  const modelFilterInput = document.getElementById('modelFilter');
  if (modelFilterInput) {
    modelFilterCombobox = createSearchableCombobox({
      attachMode: true,
      inputId: 'modelFilter',
      dropdownId: 'modelFilterDropdown',
      initialValue: filters.model,
      initialLabel: modelFilterInputValueFromFilterValue(filters.model),
      getOptions: () => {
        const allLabel = getModelAllLabel();
        return [{ value: 'all', label: allLabel }].concat(
          modelFilterOptions.map(m => ({ value: m, label: m }))
        );
      },
      onSelect: (value) => {
        filters.model = value;
        if (typeof saveChannelsFilters === 'function') saveChannelsFilters();
        filterChannels();
      }
    });
  }

  // 渠道名称筛选 combobox
  const searchInput = document.getElementById('searchInput');
  if (searchInput) {
    const allLabel = getChannelNameAllLabel();
    channelNameCombobox = createSearchableCombobox({
      attachMode: true,
      inputId: 'searchInput',
      dropdownId: 'searchInputDropdown',
      initialValue: filters.search,
      initialLabel: filters.search || allLabel,
      getOptions: () => {
        const nameSet = new Set();
        const typeFilter = (filters && filters.channelType) ? filters.channelType : 'all';
        channels.forEach(ch => {
          if (typeFilter !== 'all' && (ch.channel_type || 'anthropic') !== typeFilter) return;
          if (ch.name) nameSet.add(ch.name);
        });
        return [{ value: '', label: allLabel }].concat(
          Array.from(nameSet).sort().map(name => ({ value: name, label: name }))
        );
      },
      onSelect: (value) => {
        filters.search = value;
        if (typeof saveChannelsFilters === 'function') saveChannelsFilters();
        filterChannels();
      }
    });
  }

  // 筛选按钮：手动触发筛选
  document.getElementById('btn_filter').addEventListener('click', () => {
    if (typeof saveChannelsFilters === 'function') saveChannelsFilters();
    filterChannels();
  });

  // 解除冷却按钮
  const clearCooldownBtn = document.getElementById('btn_clear_cooldown');
  if (clearCooldownBtn) {
    clearCooldownBtn.addEventListener('click', openClearCooldownModal);
  }

  // 解除冷却模态框关闭按钮
  document.querySelectorAll('[data-action="close-clear-cooldown-modal"]').forEach(btn => {
    btn.addEventListener('click', closeClearCooldownModal);
  });

  // 全选/取消全选
  const btnSelectAllCooldown = document.getElementById('btnSelectAllCooldown');
  const btnDeselectAllCooldown = document.getElementById('btnDeselectAllCooldown');
  if (btnSelectAllCooldown) {
    btnSelectAllCooldown.addEventListener('click', () => toggleAllCooldownSelection(true));
  }
  if (btnDeselectAllCooldown) {
    btnDeselectAllCooldown.addEventListener('click', () => toggleAllCooldownSelection(false));
  }

  // 确定解除按钮
  const btnConfirmClearCooldown = document.getElementById('btnConfirmClearCooldown');
  if (btnConfirmClearCooldown) {
    btnConfirmClearCooldown.addEventListener('click', confirmClearCooldown);
  }

  // 点击模态框背景关闭
  const modal = document.getElementById('clearCooldownModal');
  if (modal) {
    modal.addEventListener('click', (e) => {
      if (e.target === modal) closeClearCooldownModal();
    });
  }

  // ESC键关闭
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      const modal = document.getElementById('clearCooldownModal');
      if (modal && modal.classList.contains('show')) {
        closeClearCooldownModal();
      }
    }
  });
}

// ==================== 解除冷却功能 ====================
let selectedCooldownChannelIds = new Set();

function openClearCooldownModal() {
  // 获取所有处于冷却中的渠道
  const cooldownChannels = channels.filter(c => c.cooldown_remaining_ms > 0);
  const countEl = document.getElementById('cooldownChannelCount');
  const listEl = document.getElementById('cooldownChannelList');

  if (countEl) countEl.textContent = cooldownChannels.length;

  if (cooldownChannels.length === 0) {
    listEl.innerHTML = `<div class="cooldown-channel-empty">${window.t('channels.noCooldownChannels')}</div>`;
  } else {
    selectedCooldownChannelIds.clear();
    // 默认全选
    cooldownChannels.forEach(c => selectedCooldownChannelIds.add(String(c.id)));
    renderCooldownChannelList(cooldownChannels);
  }

  const modal = document.getElementById('clearCooldownModal');
  if (modal) modal.classList.add('show');
}

function closeClearCooldownModal() {
  const modal = document.getElementById('clearCooldownModal');
  if (modal) modal.classList.remove('show');
  selectedCooldownChannelIds.clear();
}

function renderCooldownChannelList(cooldownChannels) {
  const listEl = document.getElementById('cooldownChannelList');
  if (!listEl) return;

  const formatDuration = (ms) => {
    const s = Math.ceil(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const secs = s % 60;
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${secs}s`;
    return `${secs}s`;
  };

  listEl.innerHTML = cooldownChannels.map(channel => {
    const id = String(channel.id);
    const checked = selectedCooldownChannelIds.has(id) ? 'checked' : '';
    const durationText = formatDuration(channel.cooldown_remaining_ms || 0);
    const typeLabel = (channel.channel_type || 'anthropic').toUpperCase();
    return `
      <div class="cooldown-channel-item">
        <input type="checkbox" data-channel-id="${id}" ${checked} onchange="toggleCooldownChannel('${id}')">
        <div class="cooldown-channel-info">
          <div class="cooldown-channel-name">${escapeHtml(channel.name)}</div>
          <div class="cooldown-channel-meta">ID: ${channel.id} · ${typeLabel}</div>
        </div>
        <span class="cooldown-channel-badge">${durationText}</span>
      </div>
    `;
  }).join('');
}

// 全局函数供内联 onclick 调用
window.toggleCooldownChannel = function(channelId) {
  if (selectedCooldownChannelIds.has(channelId)) {
    selectedCooldownChannelIds.delete(channelId);
  } else {
    selectedCooldownChannelIds.add(channelId);
  }
};

function toggleAllCooldownSelection(select) {
  if (select) {
    channels.forEach(c => {
      if (c.cooldown_remaining_ms > 0) selectedCooldownChannelIds.add(String(c.id));
    });
  } else {
    selectedCooldownChannelIds.clear();
  }
  // 重新渲染
  const cooldownChannels = channels.filter(c => c.cooldown_remaining_ms > 0);
  renderCooldownChannelList(cooldownChannels);
}

// 更新解除冷却按钮的红点提示
function updateClearCooldownBadge() {
  const btn = document.getElementById('btn_clear_cooldown');
  if (!btn) return;

  const hasCooldown = channels.some(c => c.cooldown_remaining_ms > 0);
  const existingDot = btn.querySelector('.cooldown-dot');

  if (hasCooldown && !existingDot) {
    const dot = document.createElement('span');
    dot.className = 'cooldown-dot';
    btn.appendChild(dot);
  } else if (!hasCooldown && existingDot) {
    existingDot.remove();
  }
}

async function confirmClearCooldown() {
  if (selectedCooldownChannelIds.size === 0) {
    if (window.showError) window.showError(window.t('channels.clearCooldownNoneSelected'));
    return;
  }

  const channelIds = Array.from(selectedCooldownChannelIds);
  let successCount = 0;
  let failCount = 0;

  // 显示加载状态
  const btn = document.getElementById('btnConfirmClearCooldown');
  const originalText = btn.textContent;
  btn.disabled = true;
  btn.textContent = window.t('common.processing') || '处理中...';

  for (const channelId of channelIds) {
    try {
      const resp = await fetchWithAuth(`/admin/channels/${channelId}/cooldown`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ duration_ms: 0 })
      });
      if (resp.ok) {
        successCount++;
      } else {
        failCount++;
      }
    } catch (err) {
      failCount++;
    }
  }

  btn.disabled = false;
  btn.textContent = originalText;

  // 刷新数据
  if (typeof loadChannels === 'function') {
    await loadChannels();
  }

  if (failCount === 0) {
    if (window.showSuccess) {
      window.showSuccess(window.t('channels.clearCooldownSuccess', { count: successCount }));
    }
    closeClearCooldownModal();
  } else if (successCount === 0) {
    if (window.showError) {
      window.showError(window.t('channels.clearCooldownFailed'));
    }
  } else {
    if (window.showNotification) {
      window.showNotification(
        window.t('channels.clearCooldownPartial', { success: successCount, fail: failCount }),
        'warning'
      );
    }
    // 部分成功，刷新列表
    openClearCooldownModal();
  }
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}
