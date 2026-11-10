# PZLink — plano

Objetivo: jogar Project Zomboid entre Windows e Linux sem VPS e sem abrir portas. Só o anfitrião tem conta no Tailscale.

## Decisões
- **Go + tsnet** (Tailscale embutido): gera um executável único, sem instalar o Tailscale e sem precisar de administrador.
- **Interface:** página web local, aberta no navegador. Assim não precisa de cgo nem de bibliotecas gráficas e compila igual nos dois sistemas.
- **Convite:** chave de autenticação (reutilizável e efêmera) + nome do anfitrião na rede, codificados num texto só.
- **Proxy UDP:** o amigo conecta em 127.0.0.1:16261. Do lado do anfitrião, o PZLink entrega para 127.0.0.1:16261. Portas padrão: 16261 e 16262.
- **Medição:** ping disco do Tailscale (mostra se a conexão é direta ou via relay), mais média, variação e perda de pacotes.

## Fases
- [x] 0. Pasta, README e repositório no GitHub
- [x] 1. Núcleo: tsnet + proxy UDP (anfitrião e amigo). Testes automáticos do repasse, do convite e do ping (`go test`)
- [x] 2. Interface web local com convite e painel de ping
- [x] 3. Builds para Windows e Linux + release automática no GitHub (tag `v*`; tag com `-` vira pré-lançamento)
- [x] 4. Teste real com um amigo (2026-11-08): túnel funcionou; bloqueios eram do jogo (convite Steam na B42, Steam Relay, modo debug)
- [x] 5. Auditoria de segurança e preparo para a comunidade (2026-11-09): ver "Auditoria" abaixo
- [ ] 6. Testar a receita de ACL com `tag:pzguest` numa partida real
- [ ] 7. Confirmar o fim da queda por modo debug no PC do amigo

## Auditoria (2026-11-09)
Corrigido:
- dependência vulnerável `klauspost/compress` (GO-2026-6865), via govulncheck
- convite permitia se passar pelo anfitrião (só nome); agora fixa a chave pública do aparelho
- chave do anfitrião ia em todo convite; agora há chave opcional só para convidados
- convite ficava salvo em disco no PC dos amigos; não é mais salvo (e é apagado de configs antigas)
- tsnet enviava logs para o Tailscale; desligado (`TS_NO_LOGS_NO_SUPPORT`)
- log local guardava IPs públicos; agora esconde chaves, convites e IPs públicos
- sessões ilimitadas por convidado; agora 48 por convidado / 512 no total, descartando a mais antiga
- detecção do servidor ocupava a porta 16261 a cada segundo (podia impedir o jogo de abrir); agora lê a tabela de portas do sistema
- convite aceitava portas e nomes arbitrários; agora validado
- interface sem CSP e afins; agora com CSP, nosniff, no-referrer, no-store
- CI com vet, testes (-race no Linux), govulncheck semanal; release com SHA256SUMS e atestado de origem; ações fixadas por hash; Dependabot

## Riscos a confirmar no teste real
- O servidor do jogo vê todos os amigos com o IP 127.0.0.1 (portas diferentes). Se o jogo bloquear várias conexões do mesmo IP, isso precisa de outra solução.
- Confirmar se o jogo usa só as portas 16261/16262 numa conexão por IP com a Steam ligada.
