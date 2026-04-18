package savage.proxybridge;

import com.mojang.authlib.GameProfile;
import com.mojang.authlib.properties.Property;
import com.mojang.authlib.properties.PropertyMap;
import com.google.common.collect.LinkedHashMultimap;
import io.netty.buffer.Unpooled;
import net.fabricmc.api.ModInitializer;
import net.fabricmc.fabric.api.networking.v1.ServerLoginConnectionEvents;
import net.fabricmc.fabric.api.networking.v1.ServerLoginNetworking;
import net.minecraft.network.FriendlyByteBuf;
import net.minecraft.network.chat.Component;
import savage.proxybridge.mixin.ServerLoginNetworkHandlerAccessor;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.UUID;

public class SavageProxyBridge implements ModInitializer {
    @Override
    public void onInitialize() {
        SavageProxyConfig.LOGGER.info("Savage Proxy Bridge initialized (26.1.x)");

        // Send forwarding challenge when a player begins login
        ServerLoginConnectionEvents.QUERY_START.register((handler, server, sender, synchronizer) -> {
            sender.sendPacket(SavageProxyConfig.FORWARDING_CHANNEL, new FriendlyByteBuf(Unpooled.buffer()));
        });

        // Handle the signed response from the proxy
        ServerLoginNetworking.registerGlobalReceiver(SavageProxyConfig.FORWARDING_CHANNEL, (server, handler, understood, buf, synchronizer, responseSender) -> {
            if (!understood) {
                handler.disconnect(Component.literal("This server requires a proxy connection."));
                return;
            }

            try {
                // Read the 32-byte HMAC-SHA256 signature
                byte[] signature = new byte[32];
                buf.readBytes(signature);

                // Read the remaining data payload
                byte[] data = new byte[buf.readableBytes()];
                buf.readBytes(data);

                // Verify signature integrity
                if (!verifySignature(signature, data)) {
                    SavageProxyConfig.LOGGER.warn("Invalid proxy signature from connection!");
                    handler.disconnect(Component.literal("Invalid proxy signature."));
                    return;
                }

                // Unpack the verified forwarding data
                FriendlyByteBuf dataBuf = new FriendlyByteBuf(Unpooled.wrappedBuffer(data));
                dataBuf.readVarInt(); // version (unused)
                dataBuf.readUtf();    // remoteAddr (unused)
                UUID playerUuid = dataBuf.readUUID();
                String playerName = dataBuf.readUtf();

                // Build the properties map first using raw Guava (ensures mutability during building)
                LinkedHashMultimap<String, Property> mutableMap = LinkedHashMultimap.create();
                int propertyCount = dataBuf.readVarInt();
                for (int i = 0; i < propertyCount; i++) {
                    String name = dataBuf.readUtf();
                    String value = dataBuf.readUtf();
                    String sig = dataBuf.readBoolean() ? dataBuf.readUtf() : null;
                    mutableMap.put(name, new Property(name, value, sig));
                }

                // Create the Authlib wrapper and the GameProfile record
                PropertyMap properties = new PropertyMap(mutableMap);
                GameProfile profile = new GameProfile(playerUuid, playerName, properties);

                dataBuf.release();

                // 4. Inject the verified profile into the login handler
                ((ServerLoginNetworkHandlerAccessor) handler).setAuthenticatedProfile(profile);
                SavageProxyConfig.LOGGER.info("Proxy forwarding verified: {} ({})", playerName, playerUuid);

            } catch (Exception e) {
                SavageProxyConfig.LOGGER.error("Failed to process proxy forwarding", e);
                handler.disconnect(Component.literal("Forwarding error."));
            }
        });
    }

    private boolean verifySignature(byte[] signature, byte[] data) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(SavageProxyConfig.FORWARDING_SECRET.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            return MessageDigest.isEqual(signature, mac.doFinal(data));
        } catch (Exception e) {
            SavageProxyConfig.LOGGER.error("HMAC verification error", e);
            return false;
        }
    }
}
