# PZLink

[![ci](https://github.com/zecmagalhaes/pzlink/actions/workflows/ci.yml/badge.svg)](https://github.com/zecmagalhaes/pzlink/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/zecmagalhaes/pzlink?include_prereleases&label=vers%C3%A3o)](https://github.com/zecmagalhaes/pzlink/releases)
[![licença](https://img.shields.io/github/license/zecmagalhaes/pzlink?label=licen%C3%A7a)](LICENSE)

**Jogue Project Zomboid com os amigos sem VPS e sem abrir portas no roteador.**

O PZLink liga o PC de quem entra ao PC de quem hospeda, como se todos estivessem na mesma rede de casa.

- Sem VPS, sem abrir portas, funciona até com NAT duplo e CGNAT.
- Windows e Linux jogam juntos.
- **Só quem hospeda cria uma conta** (grátis, no Tailscale). Quem entra não cria conta nenhuma.
- Um arquivo só: não instala nada e não precisa de administrador.
- Mostra ping, variação e perda de pacotes, e avisa se a conexão está boa para jogar.
- **Lobby 3D:** os sobreviventes conectados aparecem lado a lado num acampamento, com apelido e ping sobre a cabeça.

> 🧪 **Beta.** Já testado numa partida real (Build 42.21, Windows). Problemas? Abra uma [issue](../../issues/new/choose).

<details>
<summary><b>English</b></summary>

PZLink lets you play Project Zomboid with friends without renting a VPS or forwarding ports. It embeds [Tailscale](https://tailscale.com) (via `tsnet`) in a single executable for Windows and Linux. Only the host needs a free Tailscale account; friends join with an invite code. Friends point the game at `127.0.0.1:16261` and PZLink relays the UDP traffic to the host over an encrypted WireGuard tunnel. The UI is in Portuguese. See [SECURITY.md](SECURITY.md) and [docs/SEGURANCA.md](docs/SEGURANCA.md) for the threat model.
</details>

---

## Entrar na partida de um amigo

1. Baixe o arquivo do seu sistema em [**Releases**](../../releases/latest):
   - Windows: `pzlink-windows.exe`
   - Linux: `pzlink-linux`
2. Abra o programa.
   - **Windows:** se aparecer "O Windows protegeu o computador", clique em **Mais informações** e depois em **Executar assim mesmo**. O aviso aparece porque o programa não tem assinatura paga ([como conferir o arquivo](#conferir-o-download)).
   - **Linux:** `chmod +x pzlink-linux && ./pzlink-linux`
3. O navegador abre o PZLink. Escolha um apelido, cole o **código de convite** e clique em **Entrar**.
4. Espere ficar **verde**.
5. No jogo, em **Entrar**, adicione um servidor com:
   - **IP:** `127.0.0.1`
   - **Porta:** `16261`
   - **Usar Steam Relay:** **desmarcado**
6. Crie a sua conta no servidor (usuário e senha que você inventar) e entre.

Deixe o PZLink aberto enquanto joga.

---

## Hospedar uma partida

### Uma vez só: criar a chave do Tailscale
1. Crie uma conta grátis em <https://login.tailscale.com>. **Use uma conta só para o PZLink**, sem outros aparelhos: quem tiver o convite alcança todos os aparelhos da conta. O PZLink avisa se encontrar algum.
2. Em <https://login.tailscale.com/admin/settings/keys>, clique em **Generate auth key**, marque **Reusable** e **Ephemeral** e copie a chave (`tskey-auth-...`).
3. *(Recomendado)* Gere uma **segunda chave** igual para os convidados. Assim, a sua chave principal nunca sai do seu PC.

### Toda vez que for jogar
1. No Project Zomboid, clique em **Hospedar** e abra o mundo.
2. Abra o PZLink, aba **Hospedar**, cole a chave (e a dos convidados, em "Mais seguro") e clique em **Iniciar**. O PZLink guarda as chaves para as próximas vezes.
3. Clique em **Copiar convite** e mande para os amigos. **O convite muda a cada vez que você hospeda.**
4. **Build 42:** convide cada amigo pela Steam. Com o jogo aberto, aperte **Shift+Tab**, clique com o botão direito no amigo e escolha **Convidar para jogar**. O amigo não precisa aceitar: o convite só libera a entrada dele no servidor.

> ⚠️ Trate o convite como uma senha. Se vazar, apague a chave usada nele em <https://login.tailscale.com/admin/settings/keys>.

---

## Lobby 3D

No topo do PZLink aparece um acampamento ao entardecer com os sobreviventes da partida: você no centro, o anfitrião e os amigos ao lado, cada um com apelido, nome do personagem e ping.

- **Monte o seu sobrevivente:** escolha o sexo (ou deixe aleatório) e a **profissão do jogo**, que define a roupa: policial de farda, médico de jaleco, bombeiro, chef... A ficha mostra as habilidades iniciais em barras, como na criação de personagem do jogo.
- O rosto e o cabelo são sorteados a partir do apelido. Todo mundo vê o mesmo boneco para a mesma pessoa.
- Cada placa mostra apelido, se o jogador está online e o ping. Passe o mouse para ver a ficha da profissão.
- **Os modelos e as texturas vêm da sua instalação do Project Zomboid**, lidos na hora. Nenhum arquivo do jogo está neste repositório nem no executável. Sem o jogo instalado, o lobby mostra um aviso e o resto do PZLink funciona normalmente.
- Se o jogo estiver numa pasta fora do padrão da Steam, defina a variável `PZ_HOME` com a pasta que contém `media`.
- Quer ver o lobby cheio sem ninguém conectado? Abra o PZLink e acrescente `?demo=4` ao endereço no navegador.

## Problemas comuns

| O que aparece | O que fazer |
|---|---|
| **NotInvited** / "não convidado" | O anfitrião precisa convidar o amigo pela Steam (passo 4 acima), de novo a cada vez que hospeda. |
| **Conecta e cai em poucos segundos** | Desmarque **Usar Steam Relay** na conta do servidor. Com ela marcada, o jogo ignora o PZLink. |
| **"Não é permitida uma ligação ao depurador para não administradores"** | O jogo do amigo está em modo debug. Na Steam: Project Zomboid → Propriedades → Geral → apague `-debug` das opções de inicialização e reabra o jogo. |
| **InvalidServerPassword** | A senha do servidor está errada. Com o PZLink ela não é necessária: só entra quem tem o convite. Dá para deixar vazia nas configurações do servidor. |
| **Versão diferente / mods** | Todos precisam da mesma versão do jogo e dos mesmos mods. Preencha `WorkshopItems` nas configurações do servidor para os mods baixarem sozinhos. |
| **Ping alto, "via relay"** | O Tailscale não conseguiu ligar os PCs diretamente. Ainda dá para jogar. Ligar o UPnP no roteador costuma ajudar. |
| **"A porta 16261 já está em uso"** (amigo) | Feche o servidor do jogo ou outro PZLink aberto no PC do amigo. |

**Que ping é bom?** Até 80 ms é ótimo, até 150 ms é jogável. Acima disso, ou com perda de pacotes acima de 1%, os zumbis começam a "teleportar".

**Dica de segurança para o anfitrião:** nas configurações do servidor, desligue **Público** e **UPnP**. Com o PZLink, o servidor não precisa aparecer na internet.

---

## Segurança

**O PZLink deixa o meu PC vulnerável?** Não. Em resumo:

- **Não cria placa de rede virtual**, ao contrário do Hamachi e do Radmin. Ninguém da rede enxerga as suas pastas compartilhadas nem outros serviços do PC.
- Do lado de quem entra, só as portas do jogo ficam abertas, e só em `127.0.0.1`.
- Tráfego **criptografado de ponta a ponta** (WireGuard).
- **Ninguém se passa pelo anfitrião:** o convite leva a identidade criptográfica do aparelho dele.
- **Nenhum log sai do seu PC**, nem para o Tailscale. O log local esconde chaves e IPs públicos.
- A interface só responde em `http://127.0.0.1:<porta>` e bloqueia sites de fora.

Detalhes e riscos conhecidos: [docs/SEGURANCA.md](docs/SEGURANCA.md). Achou uma falha? Veja o [SECURITY.md](SECURITY.md) (relato privado).

### Conferir o download
Cada release tem um `SHA256SUMS.txt` e um atestado de origem gerado pelo GitHub.

```powershell
# Windows (PowerShell): compare com a linha do SHA256SUMS.txt
Get-FileHash .\pzlink-windows.exe -Algorithm SHA256
```
```bash
# Linux
sha256sum -c SHA256SUMS.txt --ignore-missing
# Com o GitHub CLI: confirma que o arquivo saiu deste repositório
gh attestation verify pzlink-linux --repo zecmagalhaes/pzlink
```

---

## Como funciona

```
Jogo do amigo ──UDP──> PZLink do amigo (127.0.0.1:16261/16262)
                            │  rede Tailscale (WireGuard)
                            ▼
                       PZLink do anfitrião ──UDP──> servidor do jogo (127.0.0.1)
```

Cada PZLink entra na rede Tailscale do anfitrião com a chave do convite, como aparelho **efêmero**, que some quando o programa fecha. O PZLink do amigo recebe o tráfego do jogo e manda pelo túnel; o do anfitrião entrega ao servidor do jogo, no mesmo PC.

## Compilar

Precisa do [Go](https://go.dev/dl/) (versão no `go.mod`).

```bash
go test ./...
go build -trimpath -o pzlink .
GOOS=windows GOARCH=amd64 go build -trimpath -o pzlink-windows.exe .
```

Quer ajudar? Veja o [CONTRIBUTING.md](CONTRIBUTING.md).

## Licença e avisos

[MIT](LICENSE). Inclui o [three.js](https://threejs.org) (MIT, licença em `web/vendor/three/LICENSE`). O PZLink não tem ligação com a The Indie Stone (Project Zomboid) nem com a Tailscale Inc. "Tailscale" e "Project Zomboid" são marcas dos respectivos donos.
