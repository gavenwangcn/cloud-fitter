export default [
  {
    path: '/',
    component: '../layouts/basic',
    routes: [
      {
        path: '/',
        redirect: '/ecs',
      },
      {
        path: '/billing',
        component: './billing',
      },
      {
        path: '/ecs',
        component: './ecs',
      },
      {
        path: '/rds',
        component: './rds',
      },
      {
        path: '/dcs',
        component: './dcs',
      },
      {
        path: '/dms',
        component: './dms',
      },
      {
        path: '/cce',
        component: './cce',
      },
      {
        path: '/config',
        component: './config',
      },
      {
        path: '/system',
        component: './system',
      },
      {
        path: '/cmdb-sync',
        component: './cmdb-sync',
      },
      {
        path: '/home',
        redirect: '/ecs',
      },
      {
        component: './exception/404',
      },
    ],
  },
  {
    component: './exception/404',
  },
];
