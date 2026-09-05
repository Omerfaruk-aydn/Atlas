# Claude Pro/Max aboneliğiyle giriş: ne çalışıyor, ne çalışmıyor, neden

Bu belge `atlas-agent login claude` ve `claude` sağlayıcısının hangi noktaya kadar
çalıştığını, hangi duvara çarptığını ve o duvarın neden aşılmadığını kayda geçirir.
Amaç, ileride aynı yolu tekrar yürüyen birinin baştan başlamasını önlemek.

**Özet:** Giriş (kimlik doğrulama) çalışıyor. Model çağrısı çalışmıyor ve
Atlas-Agent'ın kod tarafında düzeltilebilecek bir sebepten dolayı çalışmıyor —
Anthropic, abonelik token'ıyla yapılan çıkarım (inference) isteklerini kendi
resmi istemcisiyle sınırlıyor.

---

## 1. Hedef

Kullanıcının mevcut Claude Pro/Max aboneliğini, ayrı bir kullandıkça-öde Anthropic
API anahtarı almadan Atlas-Agent içinden kullanabilmek. Bu, Copilot ve Antigravity
için zaten yapılmış olan "abonelik OAuth'u" deseninin Claude karşılığı.

## 2. Ulaşılan nokta

| Aşama | Durum |
| --- | --- |
| OAuth grant elde etme | ✅ Çalışıyor (mevcut Claude Code oturumundan devralma ile) |
| Token'ı saklama | ✅ Çalışıyor |
| Token yenileme (refresh) | ✅ Çalışıyor |
| Yenilenen token'ı Claude Code ile senkronlama | ✅ Çalışıyor |
| Model çağrısı (`/v1/messages`) | ❌ Anthropic tarafından reddediliyor (429) |

## 3. Giriş nasıl çözüldü

İlk yaklaşım, tarayıcı üzerinden kendi OAuth akışımızı kurmaktı. Bu yol
denendi ve terk edildi (ayrıntı için bkz. bölüm 5). Çalışan yaklaşım şu:

Resmi Claude Code CLI, abonelik grant'ini düz JSON olarak
`~/.claude/.credentials.json` altında (`claudeAiOauth` bloğu) tutuyor. Makinede
zaten geçerli bir Claude Code oturumu varsa, o grant devralınabilir — tarayıcı
onay ekranına hiç girmeden. Aynı yaklaşımı `oh-my-pi` de kullanıyor
("inherits authentication from existing dotfiles").

İlgili kod: [`internal/oauth/claude/existing.go`](../internal/oauth/claude/existing.go)

Bu yol seçilirken dikkat edilen üç nokta:

**Ölü grant'i içeri almamak.** Access token'ın süresinin dolması normaldir; refresh
token onu yeniler. Ama *refresh* token'ın süresi dolduysa grant tamamen ölüdür ve
onu içeri alıp "giriş başarılı" demek, ilk istekte 401 olarak geri döner. Bu yüzden
`refreshTokenExpiresAt` kontrol edilir ve ölü grant `ErrExistingLoginExpired` ile
reddedilir.

**Refresh'i gerçekten bağlamak.** `internal/config/store.go` içindeki `exchange()`
fonksiyonu yalnızca copilot/chatgpt/antigravity'yi tanıyordu; `claude` listede
olmadığı için token bayatladığında yenileme hiç denenmiyor, doğrudan 401 alınıyordu.

**Rotation'ın Claude Code'u düşürmesini engellemek.** Anthropic refresh sırasında
refresh token'ı *rotate* ediyor. Grant Claude Code ile paylaşıldığı için,
Atlas-Agent yenileme yaptığında Claude Code'un elindeki token ölür ve kullanıcı asıl
aracından çıkış yemiş olur. Bunu önlemek için yenilenen çift, dosyadaki token
gerçekten bizim harcadığımızsa, atomik olarak geri yazılır.

İlgili kod: [`internal/oauth/claude/writeback.go`](../internal/oauth/claude/writeback.go)

## 4. Duvar: abonelik token'ıyla çıkarım

Giriş tamamlandıktan sonra `POST https://api.anthropic.com/v1/messages` isteği
şu cevabı veriyor:

```json
{"type":"error","error":{"type":"rate_limit_error","message":"Error"},"request_id":"..."}
```

HTTP 429. Mesajın gövdesi kasıtlı olarak boş: gerçek bir kota aşımı normalde
"this request would exceed your rate limit..." gibi açıklayıcı bir metin döner.

**Ayırt edici test.** Aynı hesapla, aynı dakikada, resmi CLI çalıştırıldı:

```bash
claude -p "reply with exactly: ok"
# -> ok   (exit 0)
```

Resmi istemci sorunsuz cevap verirken bizim isteğimiz 429 alıyor. Yani kota dolu
değil; istek, istemci kimliğine bakılarak reddediliyor.

**Aradaki fark ne?** Resmi istemci kendisini iki şeyle tanıtıyor:

- Sistem promptunun ilk satırı: `You are Claude Code, Anthropic's official CLI for Claude.`
- `User-Agent: claude-cli/<sürüm> (external, cli)`

Bu iki değer, `oh-my-pi` projesinde de birebir aynı şekilde bulunuyor — dosyanın
adı doğrudan `packages/ai/src/providers/claude-code-fingerprint.ts` ("parmak izi").
`oh-my-pi`'de abonelikle çıkarımın çalışmasının sebebi budur.

## 5. Neden burada duruyoruz

Bu duvarı aşmanın bilinen tek yolu, isteği Anthropic'in kendi resmi istemcisi gibi
göstermek. Bu iki şeyi ayırmak gerekiyor:

- **Kullanıcının kendi kimlik bilgisini kendi makinesinde kullanması** — meşru.
  Bu belgede anlatılan giriş/devralma tarafı tam olarak budur ve uygulanmıştır.
- **Sağlayıcının sunucusuna, onun erişim kontrolünü aşmak için başka bir
  istemci olduğunu bildirmek** — kimlik taklidi. Bu, bilinçli olarak
  uygulanmamıştır.

Bu ayrım sağlayıcıdan bağımsızdır; aynı gerekçe OpenAI veya Google için de geçerli
olurdu. Karar teknik bir kısıt değil, bilinçli bir sınırdır ve kodda da açıkça
belgelenmiştir:
[`internal/deps/atlas-llm/providers/claude/claude.go`](../internal/deps/atlas-llm/providers/claude/claude.go)

## 6. Çalışan alternatifler

Claude modellerini Atlas-Agent içinde bugün kullanmak için:

1. **Anthropic API anahtarı + `anthropic` sağlayıcısı.** Tam destekli, kısıt yok.
   Kullandıkça öde.
2. **Zaten çalışan abonelik entegrasyonları:** GitHub Copilot, Google Antigravity.

`claude` sağlayıcısı ve `login claude` komutu kodda kalır: giriş/token yönetimi
doğru çalışıyor ve Anthropic ileride üçüncü taraf istemcilere abonelik çıkarımı
açarsa, geriye yalnızca kapının kalkması kalır.

## 7. Yol boyunca düzeltilen gerçek hatalar

Bu araştırma, birbirinden bağımsız birkaç somut hatayı ortaya çıkardı:

| Hata | Yer |
| --- | --- |
| `claude` için OAuth refresh hiç bağlı değildi | `internal/config/store.go` |
| Ölü grant "giriş başarılı" olarak raporlanıyordu | `internal/oauth/claude/existing.go` |
| Refresh rotation'ı Claude Code oturumunu düşürüyordu | `internal/oauth/claude/writeback.go` |
| `WithName` yok sayılıyor, `provider.Name()` sabit `"anthropic"` dönüyordu | `internal/deps/atlas-llm/providers/anthropic/anthropic.go` |
| Base URL'de çift `/v1` (`/v1/v1/messages`) | `internal/deps/atlas-llm/providers/claude/claude.go` |
| `api_endpoint` hâlâ `claude.ai`'ye bakıyordu | `configs/claude.json` |

`WithName` hatası özellikle dikkat çekici: `languageModel` yapılandırılmış adı
kullanırken `provider.Name()` sabit değeri döndürüyordu, yani ikisi çelişiyordu.
Anthropic-şekilli her dağıtımı (Vertex, Bedrock, claude) etkiliyordu.

## 8. Metodoloji notu: değerler nasıl doğrulandı

Bu iş boyunca üç ayrı tahmin turu yapıldı ve her biri farklı bir noktada reddedildi
— her seferinde bir öncekinden *daha ileri* gidildiği için yanlış varsayım
kolay fark edilmiyordu:

1. OIDC şeklinde scope listesi (`openid profile email ...`) → `Unknown scope: openid`
2. Makul görünen ama yanlış `redirect_uri` / token endpoint çifti → `Invalid request format`
3. Doğru endpoint'ler, ama fazladan istenen `org:create_api_key` scope'u → yine reddedildi

Tahmin turlarını bitiren şey, resmi CLI'nin npm paketindeki derlenmiş binary'den
(`@anthropic-ai/claude-code-win32-x64`) düz metin sabitlerin çıkarılması oldu.
Binary bir Bun derlemesi olduğu için URL'ler, scope'lar ve istek gövdesini kuran
fonksiyonlar okunabilir string olarak duruyor.

Doğrulanan değerler:

```
CLIENT_ID              9d1c250a-e61b-44d9-88ed-5944d1962f5e
CLAUDE_AI_AUTHORIZE_URL https://claude.com/cai/oauth/authorize
TOKEN_URL               https://platform.claude.com/v1/oauth/token
MANUAL_REDIRECT_URL     https://platform.claude.com/oauth/code/callback
loopback redirect_uri   http://localhost:<port>/callback
anthropic-beta          oauth-2025-04-20
anthropic-version       2023-06-01
```

Abonelik girişinin gerçekte tuttuğu scope kümesi (diskteki çalışan grant'ten
doğrulandı — `org:create_api_key` **yok**, o Console/API-key akışına ait):

```
user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload
```

Çıkarılacak ders: belgelenmemiş bir protokolde, hata mesajlarına bakarak tahmin
yürütmek pahalı ve yanıltıcı. Kaynak elde varsa (burada: kamuya açık npm paketi)
önce oradan doğrulamak çok daha hızlı.
