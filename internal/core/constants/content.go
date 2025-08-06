package constants

const (
	DefaultContentTypeJSON = ContentTypeJSON

	// Standard Content Types
	ContentTypeJSON           = "application/json"
	ContentTypeText           = "text/plain"
	ContentTypeHTML           = "text/html"
	ContentTypeXML            = "application/xml"
	ContentTypeJavaScript     = "application/javascript"
	ContentTypeCSS            = "text/css"
	ContentTypeFormURLEncoded = "application/x-www-form-urlencoded"

	// Streaming Content Types
	ContentTypeEventStream = "text/event-stream"
	ContentTypeNDJSON      = "application/x-ndjson"
	ContentTypeStreamJSON  = "application/stream+json"
	ContentTypeJSONSeq     = "application/json-seq"
	ContentTypeTextUTF8    = "text/plain; charset=utf-8"

	// Binary/Document Content Types
	ContentTypePDF            = "application/pdf"
	ContentTypeZIP            = "application/zip"
	ContentTypeGZIP           = "application/gzip"
	ContentTypeTAR            = "application/x-tar"
	ContentTypeRAR            = "application/x-rar"
	ContentType7Z             = "application/x-7z-compressed"
	ContentTypeOctetStream    = "application/octet-stream"
	ContentTypeExcel          = "application/vnd.ms-excel"
	ContentTypeWordDOCX       = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	ContentTypeOfficeDocument = "application/vnd.openxmlformats-officedocument"
	ContentTypeWordDOC        = "application/msword"
	ContentTypePowerPoint     = "application/vnd.ms-powerpoint"

	// Image Content Types
	ContentTypeImagePNG  = "image/png"
	ContentTypeImageJPEG = "image/jpeg"
	ContentTypeImageWebP = "image/webp"
	ContentTypeImageSVG  = "image/svg+xml"

	// Video Content Types
	ContentTypeVideoMP4  = "video/mp4"
	ContentTypeVideoWebM = "video/webm"

	// Media Type Prefixes
	ContentTypePrefixImage = "image/"
	ContentTypePrefixVideo = "video/"
	ContentTypePrefixAudio = "audio/"
	ContentTypePrefixFont  = "font/"
	ContentTypePrefixModel = "model/"

	// Combined Accept Headers
	AcceptJSONTextAny = "application/json, text/plain, */*"

	// Standard HTTP Headers
	HeaderContentType       = "Content-Type"
	HeaderAccept            = "Accept"
	HeaderAuthorization     = "Authorization"
	HeaderUserAgent         = "User-Agent"
	HeaderCacheControl      = "Cache-Control"
	HeaderCookie            = "Cookie"
	HeaderVia               = "Via"
	HeaderAcceptEncoding    = "Accept-Encoding"
	HeaderConnection        = "Connection"
	HeaderKeepAlive         = "Keep-Alive"
	HeaderProxyAuthenticate = "Proxy-Authenticate"
	HeaderTE                = "TE"
	HeaderTrailers          = "Trailers"
	HeaderTransferEncoding  = "Transfer-Encoding"
	HeaderUpgrade           = "Upgrade"

	// Proxy/Forwarding Headers
	HeaderXForwardedFor   = "X-Forwarded-For"
	HeaderXForwardedProto = "X-Forwarded-Proto"
	HeaderXForwardedHost  = "X-Forwarded-Host"
	HeaderXRealIP         = "X-Real-IP"
	HeaderXProxiedBy      = "X-Proxied-By"

	// Rate Limiting Headers
	HeaderXRateLimitLimit     = "X-RateLimit-Limit"
	HeaderXRateLimitRemaining = "X-RateLimit-Remaining"
	HeaderXRateLimitReset     = "X-RateLimit-Reset"
	HeaderRetryAfter          = "Retry-After"

	// Custom/API Headers
	HeaderXRequestID         = "X-Request-ID"
	HeaderXModel             = "X-Model"
	HeaderXAPIKey            = "X-Api-Key"    //nolint:gosec // We're just using the header name, not a credential
	HeaderXAuthToken         = "X-Auth-Token" //nolint:gosec // We're just using the header name, not a credential
	HeaderXServedBy          = "X-Served-By"
	HeaderProxyAuthorization = "Proxy-Authorization"

	// Cloudflare Headers
	HeaderCFConnectingIP = "CF-Connecting-IP"

	// Profile Detection Headers
	HeaderXProfileOllamaVersion = "X-ProfileOllama-Version"

	// Olla-Specific Headers
	HeaderXOllaRequestID    = "X-Olla-Request-ID"
	HeaderXOllaEndpoint     = "X-Olla-Endpoint"
	HeaderXOllaBackendType  = "X-Olla-Backend-Type"
	HeaderXOllaModel        = "X-Olla-Model"
	HeaderXOllaResponseTime = "X-Olla-Response-Time"
)
