/* global document */
document.addEventListener('DOMContentLoaded', () => {
  const names = ['vm-demo: nocx', '/workspace/demo', 'demo-host']
  const context = document.getElementById('model-context-name')
  const keyboard = document.getElementById('model-keyboard-name')
  document.querySelectorAll('[data-demo-tab]').forEach((tab) =>
    tab.addEventListener('click', () => {
      document
        .querySelectorAll('[data-demo-tab]')
        .forEach((item) => item.classList.toggle('selected', item === tab))
      const name = names[Number(tab.dataset.demoTab)]
      if (context) context.textContent = `${name}  ~`
      if (keyboard) keyboard.textContent = name
    }),
  )
  const filter = document.getElementById('demo-filter')
  const rows = [...document.querySelectorAll('.file-tree-row')]
  const empty = document.getElementById('no-files')
  filter?.addEventListener('input', () => {
    const query = filter.value.trim().toLocaleLowerCase()
    let visible = 0
    rows.forEach((row) => {
      row.hidden = !row.textContent.toLocaleLowerCase().includes(query)
      if (!row.hidden) visible += 1
    })
    if (empty) empty.hidden = visible !== 0
  })

  // All pillar interactions use fixed local fixtures: no secret is stored and no request is sent.
  const russian = document.documentElement.lang === 'ru'
  const webview = document.getElementById('webview-demo')
  const webviewStatus = document.getElementById('webview-demo-status')
  webview?.addEventListener('change', () => {
    if (webviewStatus)
      webviewStatus.textContent =
        webview.value === 'linux'
          ? russian
            ? 'Linux использует WebKitGTK.'
            : 'Linux uses WebKitGTK.'
          : russian
            ? 'macOS использует WKWebView.'
            : 'macOS uses WKWebView.'
  })
  const vaultToggle = document.getElementById('vault-demo-toggle')
  const vaultValue = document.getElementById('vault-demo-value')
  const vaultStatus = document.getElementById('vault-demo-status')
  vaultToggle?.addEventListener('click', () => {
    const reference = vaultToggle.getAttribute('aria-pressed') !== 'true'
    vaultToggle.setAttribute('aria-pressed', String(reference))
    vaultValue.textContent = reference
      ? 'DEPLOY_TOKEN=[vault reference: demo-token]'
      : 'DEPLOY_TOKEN=[secret value]'
    vaultToggle.textContent = reference
      ? russian
        ? 'Показать пример значения'
        : 'Show example value'
      : russian
        ? 'Показать сохранённый вид'
        : 'Show saved form'
    if (vaultStatus)
      vaultStatus.textContent = reference
        ? russian
          ? 'Только демонстрационная ссылка. Ничего не сохраняется.'
          : 'Demo reference only. Nothing is stored.'
        : russian
          ? 'Только вымышленное значение. Ничего не сохраняется.'
          : 'Fictional value only. Nothing is stored.'
  })
  const connection = document.getElementById('connection-demo')
  const connectionStatus = document.getElementById('connection-demo-status')
  connection?.addEventListener('change', () => {
    if (connectionStatus)
      connectionStatus.textContent =
        connection.value === 'build'
          ? russian
            ? 'Build · группа: CI · учётные данные: ссылка из vault'
            : 'Build · group: CI · credential: vault reference'
          : russian
            ? 'Staging · группа: API · учётные данные: ссылка из vault'
            : 'Staging · group: API · credential: vault reference'
  })
  const apiEnv = document.getElementById('api-demo-env')
  const apiSend = document.getElementById('api-demo-send')
  const apiResponse = document.getElementById('api-demo-response')
  apiEnv?.addEventListener('change', () => {
    if (apiResponse)
      apiResponse.textContent = `GET https://api.example.test/health\n${apiEnv.value} ${russian ? 'окружение выбрано · запрос не отправлен' : 'environment selected · no request sent'}`
  })
  apiSend?.addEventListener('click', () => {
    if (apiResponse)
      apiResponse.textContent = `200 OK (fixture)\n{ "status": "ok", "demo": true }\n${russian ? 'Сетевой запрос не отправлялся.' : 'No network request was sent.'}`
  })
})
