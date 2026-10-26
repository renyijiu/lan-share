## 改动 / Summary

<!-- 改了什么，为什么改。相关 Issue：Fixes #123 -->

## 测试 / Testing

- [ ] `gofmt -l .` 无输出，`go mod tidy -diff` 通过
- [ ] `go vet ./...` 和 `go test -race ./...` 通过
- [ ] 改动了脚本时，`python3 -m unittest discover -s scripts -p 'test_*.py'` 通过
- [ ] 新增或移除依赖时，已运行 `python3 scripts/collect-licenses.py` 并提交结果
- [ ] 用户能感知的改动已补充到 `CHANGELOG.md`

实际验证过的平台和设备 / Platforms and devices verified:

<!-- 例如：macOS 15.1 Apple Silicon，通过极米 NEW Z8X 播放 -->

## 安全相关 / Security

<!-- 是否涉及共享目录边界、只读保证、认证、端口助手或日志脱敏？如果涉及，请说明并附上测试。 -->
