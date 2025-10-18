const KEY_OUT = 'ss://YOUR_KEY';
module.exports = {
  apps: [
    {
      name: 'outline-bwg',
      script: '/bin/bash',
      args: [
        '-lc',
        '/Users/sam/.bin/http2transport -main-proxy "$KEY_OUT" -localAddr 0.0.0.0:1080 -socket-port 1079 -direct-file config/direct.txt -default main-proxy'
      ],
      env: {
        KEY_OUT: KEY_OUT
      },
      autorestart: true,
      restart_delay: 2000
    }
  ]
};