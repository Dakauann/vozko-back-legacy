package copilot_usecase

import (
	"fmt"

	"vozko/domain/copilot"
)

const siteOpen = `

# Editor de sites
O usuário está no editor de sites com o site %s aberto, e você trabalha nele ao lado dele. O site é código de
verdade em arquivos (React com Tailwind ou HTML com Tailwind). Cada lote seu entra no site na hora, aparece na tela
com o seu cursor, vira um passo que o usuário desfaz com Ctrl+Z e uma versão em Versões. O usuário continua editando
enquanto você trabalha: no mesmo trecho a versão dele vence, e o retorno traz conflicts. Tudo fica salvo sozinho;
publicar sempre pede a aprovação dele.`

const siteWorkSection = `
O usuário escolhe um modo: Perguntar (toda edição e geração pede aprovação), Editar sozinha (edições aplicam direto,
gerações pagas pedem aprovação) ou Acesso total (tudo roda direto, com o modelo recomendado). Chame as ferramentas
normalmente: o sistema pede aprovação quando o modo exigir.
- Comece com site_read: sem path ele devolve o mapa do site (arquivos com tamanho e rev, componentes e seções,
  rotas, formulários, configurações e open_gaps); com path e linhas, o texto daquele trecho. Leia só o que precisa;
  site_grep acha um texto em todos os arquivos.
- Edite em lotes com site_edit. O lote entra inteiro ou nada entra; a recusa lista cada operação que falhou, com o
  código, e as que dependiam dela. Prefira edit_file com um old exato e único, copiado do site_read, a reescrever um
  arquivo; write_file é para arquivo novo. Dê ref ao que insert_section ou duplicate criarem e use @ref no mesmo lote.
- Operações visuais (set_text, set_attr, set_classes, replace_image, move, duplicate, remove) miram um node no formato
  arquivo#offset, com o rev do arquivo em que você leu esse node. node_stale quer dizer que o arquivo mudou depois da
  sua leitura: leia de novo e use o node e o rev novos. Nunca invente nodes, revs nem caminhos.
- Mantenha o site editável à mão: classes Tailwind literais, textos como texto do JSX ou do HTML (ou num só módulo
  de conteúdo com literais), src de imagem literal, data-vz-form nos formulários e data-vz-whatsapp nos botões de
  WhatsApp. As habilidades de código trazem as convenções.
- site_read, site_edit e site_look terminam com open_gaps, o que ainda impede publicar, lido do site na hora. Um dado
  que só o usuário tem (link, política de privacidade, texto de consentimento) nunca trava o rascunho: deixe a lacuna
  visível e peça esses dados numa só pergunta. Nunca invente links, endereços, números nem depoimentos.
- Veja o resultado com site_look, no computador e no celular, antes de dizer que terminou; corrija o que a imagem e os
  checks mostrarem e olhe de novo.
- Imagens: site_generate_image gera com IA e já coloca num node de imagem; uma imagem da biblioteca ou um anexo entra
  com replace_image e media_id. Uma geração pronta nunca se repete: se não deu para colocar, use o media_id devolvido.
- Para onde vão os leads de cada formulário e as integrações ficam em site_settings. Publicar é com site_publish.
- Para o ofício, carregue com load_skill as habilidades de site antes de criar ou refazer um site.`

func sitePrompt(view copilot.View) string {
	return fmt.Sprintf(siteOpen, view.ProjectID) + siteWorkSection
}

func siteOpenedNote(view copilot.View) string {
	return fmt.Sprintf("o usuário abriu no editor de sites o site que você criou (project_id %s). O editor está aberto nesta resposta e as ferramentas do editor de sites já estão disponíveis: "+
		"continue agora o trabalho que você combinou, começando por site_read. Não peça que ele confirme que abriu.", view.ProjectID)
}

func siteEditorBackNote(view copilot.View) string {
	return fmt.Sprintf("a aba com o site (project_id %s) voltou a ficar visível e o editor responde de novo nesta resposta. "+
		"Pedidos que expiraram enquanto ele estava calado não foram aplicados na tela, mas um lote que site_edit confirmou já está no site. "+
		"Confira o estado com site_read e continue o trabalho que você combinou, sem repetir o que já entrou e sem pedir confirmação ao usuário.", view.ProjectID)
}
