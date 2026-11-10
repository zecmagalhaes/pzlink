# Política de segurança

## Como relatar uma vulnerabilidade

**Não abra uma issue pública.** Use o relato privado do GitHub:
aba **Security** → **Report a vulnerability**
(<https://github.com/zecmagalhaes/pzlink/security/advisories/new>).

Inclua o que for possível: versão do PZLink, sistema operacional, passos para reproduzir e o impacto que você enxerga. A resposta inicial deve sair em até 7 dias. Quando a correção for publicada, o relato fica público com o crédito para quem encontrou, se a pessoa quiser.

## Versões com correções de segurança

Só a versão mais recente recebe correções. Atualize sempre pela página de [Releases](https://github.com/zecmagalhaes/pzlink/releases).

## Escopo

Entram no escopo:
- o executável do PZLink e o código deste repositório;
- o processo de build e publicação (GitHub Actions).

Ficam fora do escopo:
- falhas do Tailscale (relate em <https://tailscale.com/security>);
- falhas do Project Zomboid ou do servidor do jogo;
- ataques que exigem um programa malicioso já rodando no seu PC.

O modelo de ameaças completo está em [docs/SEGURANCA.md](docs/SEGURANCA.md).

---

## Security policy (English)

Please report vulnerabilities privately through GitHub: **Security → Report a vulnerability**. Do not open public issues for security problems. Only the latest release receives fixes. See [docs/SEGURANCA.md](docs/SEGURANCA.md) for the threat model (in Portuguese).
