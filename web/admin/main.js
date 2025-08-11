const { createApp, ref, reactive, onMounted } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

// API 封装
const api = {
  // 基础请求函数
  async request(url, options = {}) {
    const token = localStorage.getItem('admin_token');
    const headers = {
      'Content-Type': 'application/json',
      ...options.headers
    };
    
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    try {
      const response = await fetch(url, {
        ...options,
        headers
      });

      if (!response.ok) {
        if (response.status === 401) {
          localStorage.removeItem('admin_token');
          localStorage.removeItem('admin_user');
          location.reload();
          return;
        }
        throw new Error(await response.text() || response.statusText);
      }

      return await response.json();
    } catch (error) {
      console.error('API Error:', error);
      ElMessage.error(error.message || 'API 调用失败');
      throw error;
    }
  },

  // 登录
  login(username, password) {
    return this.request('/api/admin/login', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    });
  },

  // 获取统计数据
  getStats() {
    return this.request('/api/admin/stats');
  },

  // 获取用户列表
  getUsers(page = 1, limit = 50) {
    return this.request(`/api/admin/users?page=${page}&limit=${limit}`);
  },

  // 获取群组列表
  getGroups(page = 1, limit = 50) {
    return this.request(`/api/admin/groups?page=${page}&limit=${limit}`);
  },

  // 获取消息统计
  getMessageStats(startDate, endDate) {
    const params = new URLSearchParams();
    if (startDate) params.append('start', startDate);
    if (endDate) params.append('end', endDate);
    return this.request(`/api/admin/message-stats?${params}`);
  },

  // 获取最近活动
  getRecentActivities() {
    return this.request('/api/admin/activities');
  },

  // 禁用用户
  banUser(userId) {
    return this.request(`/api/admin/users/${userId}/ban`, {
      method: 'POST'
    });
  },

  // 解散群组
  disbandGroup(groupId) {
    return this.request(`/api/admin/groups/${groupId}/disband`, {
      method: 'POST'
    });
  },

  // 获取系统设置
  getSettings() {
    return this.request('/api/admin/settings');
  },

  // 保存系统设置
  saveSettings(settings) {
    return this.request('/api/admin/settings', {
      method: 'PUT',
      body: JSON.stringify(settings)
    });
  }
};

// 创建 Vue 应用
const app = createApp({
  setup() {
    // 响应式状态
    const isLoggedIn = ref(false);
    const activeTab = ref('dashboard');
    const loginLoading = ref(false);
    
    // 登录表单
    const loginForm = reactive({
      username: '',
      password: ''
    });

    // 当前用户
    const currentUser = ref({
      username: '',
      id: ''
    });

    // 统计数据
    const stats = reactive({
      totalUsers: 0,
      onlineUsers: 0,
      totalGroups: 0,
      totalMessages: 0
    });

    // 用户管理
    const users = ref([]);
    const usersLoading = ref(false);

    // 群组管理
    const groups = ref([]);
    const groupsLoading = ref(false);

    // 消息统计
    const messageStats = reactive({
      todayMessages: 0,
      weekMessages: 0,
      monthMessages: 0
    });
    const dateRange = ref([]);

    // 最近活动
    const recentActivities = ref([]);

    // 系统设置
    const settings = reactive({
      systemName: 'Go-IM',
      maxGroupMembers: 500,
      messageRetentionDays: 30,
      enableRegistration: true
    });

    // 方法
    const setActiveTab = (tab) => {
      activeTab.value = tab;
      
      // 根据激活的标签页加载对应数据
      switch (tab) {
        case 'dashboard':
          loadDashboardData();
          break;
        case 'users':
          refreshUsers();
          break;
        case 'groups':
          refreshGroups();
          break;
        case 'messages':
          refreshMessageStats();
          break;
        case 'settings':
          loadSettings();
          break;
      }
    };

    // 登录处理
    const handleLogin = async () => {
      if (!loginForm.username || !loginForm.password) {
        ElMessage.warning('请输入用户名和密码');
        return;
      }

      loginLoading.value = true;
      try {
        const result = await api.login(loginForm.username, loginForm.password);
        
        // 保存登录信息
        localStorage.setItem('admin_token', result.token);
        localStorage.setItem('admin_user', JSON.stringify(result.user));
        
        currentUser.value = result.user;
        isLoggedIn.value = true;
        
        ElMessage.success('登录成功');
        
        // 加载仪表盘数据
        loadDashboardData();
      } catch (error) {
        ElMessage.error('登录失败：' + error.message);
      } finally {
        loginLoading.value = false;
      }
    };

    // 登出处理
    const handleLogout = () => {
      localStorage.removeItem('admin_token');
      localStorage.removeItem('admin_user');
      isLoggedIn.value = false;
      currentUser.value = { username: '', id: '' };
      ElMessage.success('已退出登录');
    };

    // 加载仪表盘数据
    const loadDashboardData = async () => {
      try {
        const [statsData, activities] = await Promise.all([
          api.getStats(),
          api.getRecentActivities()
        ]);
        
        Object.assign(stats, statsData);
        recentActivities.value = activities || [];
      } catch (error) {
        console.error('加载仪表盘数据失败:', error);
      }
    };

    // 刷新用户列表
    const refreshUsers = async () => {
      usersLoading.value = true;
      try {
        const result = await api.getUsers();
        users.value = result.users || [];
      } catch (error) {
        ElMessage.error('加载用户列表失败');
      } finally {
        usersLoading.value = false;
      }
    };

    // 刷新群组列表
    const refreshGroups = async () => {
      groupsLoading.value = true;
      try {
        const result = await api.getGroups();
        groups.value = result.groups || [];
      } catch (error) {
        ElMessage.error('加载群组列表失败');
      } finally {
        groupsLoading.value = false;
      }
    };

    // 刷新消息统计
    const refreshMessageStats = async () => {
      try {
        let startDate, endDate;
        if (dateRange.value && dateRange.value.length === 2) {
          startDate = dateRange.value[0].toISOString().split('T')[0];
          endDate = dateRange.value[1].toISOString().split('T')[0];
        }
        
        const result = await api.getMessageStats(startDate, endDate);
        Object.assign(messageStats, result);
      } catch (error) {
        ElMessage.error('加载消息统计失败');
      }
    };

    // 查看用户详情
    const viewUserDetails = (user) => {
      ElMessageBox.alert(
        `用户ID: ${user.id}\n用户名: ${user.username}\n昵称: ${user.nickname}\n注册时间: ${user.createdAt}`,
        '用户详情',
        { confirmButtonText: '确定' }
      );
    };

    // 禁用用户
    const banUser = async (user) => {
      try {
        await ElMessageBox.confirm(
          `确定要禁用用户 "${user.username}" 吗？`,
          '确认禁用',
          { type: 'warning' }
        );
        
        await api.banUser(user.id);
        ElMessage.success('用户已禁用');
        refreshUsers();
      } catch (error) {
        if (error !== 'cancel') {
          ElMessage.error('禁用用户失败');
        }
      }
    };

    // 查看群组详情
    const viewGroupDetails = (group) => {
      ElMessageBox.alert(
        `群组ID: ${group.id}\n群组名称: ${group.name}\n群主ID: ${group.ownerId}\n成员数量: ${group.memberCount}\n创建时间: ${group.createdAt}`,
        '群组详情',
        { confirmButtonText: '确定' }
      );
    };

    // 解散群组
    const disbandGroup = async (group) => {
      try {
        await ElMessageBox.confirm(
          `确定要解散群组 "${group.name}" 吗？`,
          '确认解散',
          { type: 'warning' }
        );
        
        await api.disbandGroup(group.id);
        ElMessage.success('群组已解散');
        refreshGroups();
      } catch (error) {
        if (error !== 'cancel') {
          ElMessage.error('解散群组失败');
        }
      }
    };

    // 加载系统设置
    const loadSettings = async () => {
      try {
        const result = await api.getSettings();
        Object.assign(settings, result);
      } catch (error) {
        ElMessage.error('加载系统设置失败');
      }
    };

    // 保存系统设置
    const saveSettings = async () => {
      try {
        await api.saveSettings(settings);
        ElMessage.success('设置已保存');
      } catch (error) {
        ElMessage.error('保存设置失败');
      }
    };

    // 检查登录状态
    const checkLoginStatus = () => {
      const token = localStorage.getItem('admin_token');
      const user = localStorage.getItem('admin_user');
      
      if (token && user) {
        try {
          currentUser.value = JSON.parse(user);
          isLoggedIn.value = true;
          // 自动加载仪表盘数据
          loadDashboardData();
        } catch (error) {
          console.error('解析用户信息失败:', error);
          handleLogout();
        }
      }
    };

    // 组件挂载时检查登录状态
    onMounted(() => {
      checkLoginStatus();
    });

    return {
      // 状态
      isLoggedIn,
      activeTab,
      loginLoading,
      loginForm,
      currentUser,
      stats,
      users,
      usersLoading,
      groups,
      groupsLoading,
      messageStats,
      dateRange,
      recentActivities,
      settings,
      
      // 方法
      setActiveTab,
      handleLogin,
      handleLogout,
      refreshUsers,
      refreshGroups,
      refreshMessageStats,
      viewUserDetails,
      banUser,
      viewGroupDetails,
      disbandGroup,
      saveSettings
    };
  }
});

// 注册 Element Plus 图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component);
}

// 使用 Element Plus
app.use(ElementPlus);

// 挂载应用
app.mount('#app'); 