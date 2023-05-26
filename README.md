# microsoft-tts-asr-go
microsoft tts and asr stream

## 准备
- 首先当然是注册微软azure，拿到token，对于个人开发者，免费时长还是足够的
- mac电脑需要配置c支持：https://github.com/microsoft/cognitive-services-speech-sdk-go/issues/66
- mac环境所需要的c相关支持都已存在本项目中，可以自行升级
- 把build.sh文件里的'绝对路径'替换成你电脑上的真实地址

## 启动example
提供了一个example来提供测试
```shell
sh ./build.sh
./fermi
```
提供了2个接口：
- 一个是普通http api生成tts，生成的wav文件会在example目录下
- tts_stream是一个eventstream的长链接http，可以实时传递音频pcm到前端，可以实现实时播放音频的需求。

![截图](https://github.com/zealerFT/microsoft-tts-asr-go/blob/main/resources/%E6%88%AA%E5%B1%8F2023-05-26%2018.30.54.png)
