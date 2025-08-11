// GO-IM Service Worker
// 提供离线支持和推送通知功能

const CACHE_NAME = 'go-im-v1.0.0';
const CACHE_URLS = [
  '/im',
  '/manifest.json',
  'https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap'
];

// 安装 Service Worker
self.addEventListener('install', (event) => {
  console.log('Service Worker 安装中...');
  
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => {
        console.log('缓存已打开');
        return cache.addAll(CACHE_URLS);
      })
      .then(() => {
        console.log('Service Worker 安装完成');
        return self.skipWaiting();
      })
  );
});

// 激活 Service Worker
self.addEventListener('activate', (event) => {
  console.log('Service Worker 激活中...');
  
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            console.log('删除旧缓存:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    }).then(() => {
      console.log('Service Worker 激活完成');
      return self.clients.claim();
    })
  );
});

// 拦截网络请求
self.addEventListener('fetch', (event) => {
  // 只处理 GET 请求
  if (event.request.method !== 'GET') {
    return;
  }
  
  // 跳过 WebSocket 和 API 请求
  if (event.request.url.includes('/ws') || 
      event.request.url.includes('/api/') ||
      event.request.url.startsWith('ws:') ||
      event.request.url.startsWith('wss:')) {
    return;
  }
  
  event.respondWith(
    caches.match(event.request)
      .then((response) => {
        // 缓存命中，返回缓存内容
        if (response) {
          return response;
        }
        
        // 尝试网络请求
        return fetch(event.request).then((response) => {
          // 检查是否是有效响应
          if (!response || response.status !== 200 || response.type !== 'basic') {
            return response;
          }
          
          // 克隆响应以便缓存
          const responseToCache = response.clone();
          
          caches.open(CACHE_NAME)
            .then((cache) => {
              cache.put(event.request, responseToCache);
            });
          
          return response;
        });
      })
      .catch(() => {
        // 网络和缓存都失败，返回离线页面
        if (event.request.destination === 'document') {
          return caches.match('/im');
        }
      })
  );
});

// 处理推送通知
self.addEventListener('push', (event) => {
  console.log('收到推送消息:', event);
  
  if (!event.data) {
    return;
  }
  
  try {
    const data = event.data.json();
    const options = {
      body: data.body || '新消息',
      icon: data.icon || '/manifest.json',
      badge: '/manifest.json',
      tag: data.tag || 'go-im-message',
      data: data.data || {},
      actions: [
        {
          action: 'reply',
          title: '回复',
          icon: '/manifest.json'
        },
        {
          action: 'view',
          title: '查看',
          icon: '/manifest.json'
        }
      ],
      requireInteraction: true,
      vibrate: [200, 100, 200]
    };
    
    event.waitUntil(
      self.registration.showNotification(data.title || 'GO-IM', options)
    );
  } catch (error) {
    console.error('处理推送消息失败:', error);
  }
});

// 处理通知点击
self.addEventListener('notificationclick', (event) => {
  console.log('通知被点击:', event);
  
  event.notification.close();
  
  if (event.action === 'reply') {
    // 处理回复操作
    event.waitUntil(
      clients.openWindow('/im#reply')
    );
  } else {
    // 默认操作：打开应用
    event.waitUntil(
      clients.matchAll({ type: 'window' }).then((clientList) => {
        // 如果已有窗口打开，聚焦到该窗口
        for (const client of clientList) {
          if (client.url.includes('/im') && 'focus' in client) {
            return client.focus();
          }
        }
        
        // 否则打开新窗口
        if (clients.openWindow) {
          return clients.openWindow('/im');
        }
      })
    );
  }
});

// 处理消息通信
self.addEventListener('message', (event) => {
  console.log('收到客户端消息:', event.data);
  
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
  
  if (event.data && event.data.type === 'GET_VERSION') {
    event.ports[0].postMessage({
      type: 'VERSION',
      version: CACHE_NAME
    });
  }
});

// 错误处理
self.addEventListener('error', (event) => {
  console.error('Service Worker 错误:', event.error);
});

self.addEventListener('unhandledrejection', (event) => {
  console.error('Service Worker 未处理的 Promise 拒绝:', event.reason);
});

console.log('Service Worker 脚本加载完成');
