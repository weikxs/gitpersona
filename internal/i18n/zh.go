package i18n

// Chinese translations live in zh_*.go files, one per part of GitPersona, each
// registering its own messages from an init function. The key is the English
// format string exactly as the code writes it; the translation keeps the same
// placeholders in the same order (TestEveryMessageHasAChineseTranslation
// checks both).
//
// Wording follows README.zh-CN.md: profile = 配置文件, pin = 钉住,
// directory rule = 目录规则, credential helper = 凭据助手, push = 推送,
// author = 作者, repository = 仓库, folder = 目录, doctor = 体检.
