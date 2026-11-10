# Como contribuir

Obrigado pelo interesse! Issues e pull requests são bem-vindos, em português ou em inglês.

## Relatar um problema

Abra uma [issue](../../issues/new/choose) e anexe o `pzlink.log`:
- Windows: `%APPDATA%\pzlink\pzlink.log`
- Linux: `~/.config/pzlink/pzlink.log`

O PZLink já esconde chaves, convites e IPs públicos nesse arquivo, mas dê uma olhada antes de enviar.

**Problemas de segurança não vão em issue pública.** Veja o [SECURITY.md](SECURITY.md).

## Rodar o código

Precisa do [Go](https://go.dev/dl/) na versão indicada no `go.mod`.

```bash
go test ./...          # testes
go vet ./...           # verificação estática
go run .               # abre o PZLink
go run golang.org/x/vuln/cmd/govulncheck@latest ./...   # vulnerabilidades conhecidas
```

## Mandar um pull request

- Uma mudança por PR, com uma descrição do problema que ela resolve.
- Rode `gofmt -w .` e os testes antes de enviar. O CI roda os mesmos comandos no Linux e no Windows.
- Mudanças no repasse de pacotes, no convite ou na interface local precisam de teste. Os testes de segurança ficam em `security_test.go`.
- Textos da interface ficam em português.

## Estrutura

| Arquivo | O que faz |
|---|---|
| `main.go` | Inicia o programa, o log e a interface |
| `app.go` | Hospedar e entrar: Tailscale, convite, ping |
| `proxy.go` | Repasse UDP entre o jogo e a rede Tailscale |
| `invite.go` | Formato e validação do convite |
| `ui.go`, `web/index.html` | Interface local no navegador |
| `logsafe.go` | Esconde chaves e IPs públicos do log |
| `gameprobe.go`, `listen_*.go` | Detecta se o servidor do jogo está aberto |
| `lobby.go`, `web/lobby.js` | Lobby 3D: lista de jogadores e cena |
| `pzasset/` | Lê os arquivos do jogo instalado: modelos `.X` → glTF, roupas, cabelos, objetos |
| `web/vendor/three/` | three.js (MIT), sem alterações |

**Nunca adicione arquivos do Project Zomboid ao repositório** (modelos, texturas, imagens tiradas deles). O PZLink lê tudo da instalação de quem usa. Os testes em `pzasset/*_local_test.go` usam o jogo instalado e são pulados quando ele não existe (como no CI); defina `PZ_MEDIA` se o seu jogo estiver em outra pasta.
