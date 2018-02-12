# HttpFtpClient
### 提供http接口，支持会话长连接
$ curl -GET 'http://127.0.0.1:10006/putfile' -d '{"ftpaddr":"127.0.0.1:21","ftpuser":"ftp","ftppasswd":"ftp","remotedir":"***","locatedir":"***","locatefile":"test.data"}'
{
  "flag": 0
}

目前仅支持put，后期完善功能点。
