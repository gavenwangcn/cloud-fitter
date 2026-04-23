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
