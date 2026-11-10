# Segurança do PZLink: o que ele protege e o que não protege

Este documento é para quem quer entender os riscos antes de usar ou de contribuir. A versão curta para jogadores está no [README](../README.md#segurança).

## Como os dados circulam

```
Jogo do amigo ──UDP──> PZLink do amigo (127.0.0.1:16261/16262)
                            │  WireGuard (criptografado de ponta a ponta)
                            ▼
                       PZLink do anfitrião ──UDP──> servidor do jogo (127.0.0.1)
```

- O Tailscale roda **dentro do programa** (biblioteca `tsnet`). Não existe placa de rede virtual: o sistema operacional não ganha rota nem interface nova.
- Os aparelhos entram na rede como **efêmeros** e saem quando o programa fecha.
- A coordenação da rede usa os servidores do Tailscale. O tráfego do jogo é criptografado de ponta a ponta: nem o Tailscale nem os servidores de relay (DERP) conseguem ler o conteúdo.

## O que fica exposto

| Quem | O que alcança |
|---|---|
| Quem tem o convite | As portas do jogo (UDP 16261/16262) no PC do anfitrião, ou seja, o servidor do jogo. Nada mais do PC do anfitrião. |
| Quem tem o convite, **se a conta do Tailscale tiver outros aparelhos** | Esses outros aparelhos, porque a regra padrão do Tailscale libera tudo entre aparelhos da mesma conta. O PZLink mostra um aviso quando detecta isso. **Use uma conta só para o PZLink.** |
| Outros amigos conectados | Nada. O PZLink do amigo não aceita conexões vindas da rede. |
| Alguém no mesmo Wi-Fi | Nada. Tudo que o PZLink abre fica em `127.0.0.1`. |
| Um site aberto no navegador | Nada. A interface só responde no endereço exato `http://127.0.0.1:<porta>` (proteção contra *DNS rebinding*) e recusa comandos de outras origens. |

## Lobby 3D

- **Lista de jogadores:** o PZLink do anfitrião publica, **só dentro do túnel** (porta TCP 7471 no endereço Tailscale dele), a lista de apelidos com status, ping, tipo de rota, sexo e profissão escolhidos. Quem está na rede da partida consegue ler essa lista.
- **Perfil dos amigos:** cada amigo envia ao anfitrião só o sexo e a profissão escolhidos. O anfitrião associa o perfil ao **endereço Tailscale de quem enviou**, autenticado pelo WireGuard, então ninguém consegue alterar o perfil de outro jogador. São guardados no máximo 64 perfis por sessão, e valores fora do padrão são descartados.
- **Lista recebida:** o PZLink dos amigos limita o que aceita (no máximo 16 jogadores, apelidos só com letras, números e hífen, ping entre 0 e 10 s), para que um anfitrião mal-intencionado não injete conteúdo na tela.
- **Modelos e texturas:** lidos da instalação do Project Zomboid do próprio jogador e servidos só para a interface local (`127.0.0.1`). Os objetos do cenário vêm de uma lista fixa; não é possível pedir outros arquivos do disco.
- **three.js:** incluído no executável (`web/vendor/three`). A página não carrega nada da internet.

## O convite

O convite é um texto com:
- uma **chave de autenticação** do Tailscale (reutilizável e efêmera);
- o nome do anfitrião na rede e a **chave pública do aparelho** dele;
- as portas do jogo.

Consequências:
- **O convite funciona como uma senha** enquanto a chave for válida. Quem tiver o convite consegue colocar aparelhos na rede do anfitrião.
- **Ninguém consegue se passar pelo anfitrião.** O PZLink do amigo só aceita o aparelho cuja chave pública está no convite. Um aparelho com o mesmo nome, criado com a chave vazada, é ignorado.
- **O convite não é salvo em disco** no PC dos amigos.
- **Chave separada para convidados (recomendado):** o anfitrião pode usar uma segunda chave só para os convites. Assim, a chave principal nunca sai do PC dele. Se um convite vazar, basta apagar a chave dos convidados no painel do Tailscale.

### Restringir os convidados com regras de acesso (avançado)

Com a chave separada, dá para ir além e limitar os convidados às portas do jogo usando as regras de acesso (ACL) do Tailscale:

1. Na política de acesso (<https://login.tailscale.com/admin/acls>), declare uma etiqueta para os convidados e libere só o jogo:
   ```jsonc
   {
     "tagOwners": { "tag:pzguest": ["autogroup:admin"] },
     "grants": [
       // seus próprios aparelhos continuam se falando normalmente
       { "src": ["autogroup:member"], "dst": ["*"], "ip": ["*"] },
       // convidados só alcançam as portas do jogo nos seus aparelhos
       { "src": ["tag:pzguest"], "dst": ["autogroup:member"], "ip": ["udp:16261-16262"] }
     ]
   }
   ```
2. Gere a chave dos convidados marcando **Tags → tag:pzguest**.

> Essa receita ainda **não foi testada numa partida real**. Se usar, conte na aba de issues se funcionou.

## O que fica salvo no PC

| Arquivo | Conteúdo |
|---|---|
| `config.json` (Windows: `%APPDATA%\pzlink`, Linux: `~/.config/pzlink`) | As chaves do anfitrião, **sem criptografia**, e o apelido. O arquivo é criado só para o seu usuário (permissão 600 no Linux; no Windows, a pasta `%APPDATA%` já é restrita ao usuário). O botão "Esquecer chave" apaga as chaves. |
| `pzlink.log` (mesma pasta) | Registro técnico, sobrescrito a cada execução. **Chaves, convites e IPs públicos são substituídos por `[oculto]`** antes de gravar, para que seja seguro anexar o arquivo numa issue. |
| Estado do Tailscale | Pasta temporária, apagada quando o programa fecha. |

## Privacidade

- O PZLink **desliga o envio de logs para o Tailscale** (`TS_NO_LOGS_NO_SUPPORT`). Nenhum registro de uso sai do seu PC.
- O PZLink não tem telemetria, não verifica atualizações sozinho e não acessa nada além do Tailscale.

## Proteções contra abuso

- **Limite de sessões:** cada convidado pode ter no máximo 48 sessões simultâneas e o total é 512. Ao atingir o limite, a sessão parada há mais tempo é descartada. Isso impede que um convidado esgote os recursos do anfitrião.
- **Convite validado:** portas abaixo de 1024, nomes de anfitrião fora do padrão e chaves malformadas são recusados.
- **Detecção do servidor do jogo sem ocupar a porta:** o PZLink consulta a tabela de portas do sistema, como o `netstat`, em vez de abrir a porta para testar.

## Cadeia de distribuição

- Os executáveis são compilados pelo **GitHub Actions** a partir do código público, nunca no PC de alguém.
- Cada release tem `SHA256SUMS.txt` e um **atestado de origem** (*build provenance*) que comprova de qual commit e de qual workflow o arquivo saiu. Para conferir:
  ```bash
  gh attestation verify pzlink-windows.exe --repo zecmagalhaes/pzlink
  ```
- As ações do GitHub ficam fixadas por hash de commit. O Dependabot propõe atualizações e o `govulncheck` roda a cada mudança e toda semana.
- Os executáveis **não têm assinatura de código paga**, por isso o Windows mostra o aviso do SmartScreen. Confira o hash ou o atestado se estiver em dúvida.

## Riscos conhecidos e aceitos

- O servidor do jogo vê todos os convidados com o IP `127.0.0.1`. Bloqueios por IP do jogo não funcionam; use os bloqueios por conta ou SteamID.
- Quem já tem um programa malicioso rodando no seu PC consegue ler o `config.json`. Isso vale para qualquer programa que guarda credenciais.
- O servidor do jogo continua com as próprias falhas. O PZLink limita **quem** chega até ele, não **o que** ele faz.
