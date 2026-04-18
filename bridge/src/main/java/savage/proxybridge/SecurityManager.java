package savage.proxybridge;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

public class SecurityManager {

    /**
     * Verifies the authenticity of the proxy data using HMAC-SHA256.
     * @param signature The 32-byte signature received from the proxy.
     * @param data The raw data payload to verify.
     * @return true if the signature is valid.
     */
    public static boolean verifySignature(byte[] signature, byte[] data) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(SavageProxyConfig.FORWARDING_SECRET.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            return MessageDigest.isEqual(signature, mac.doFinal(data));
        } catch (Exception e) {
            SavageProxyConfig.LOGGER.error("CRITICAL: HMAC verification engine failure", e);
            return false;
        }
    }
}
