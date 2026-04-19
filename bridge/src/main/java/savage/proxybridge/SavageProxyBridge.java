package savage.proxybridge;

import io.netty.buffer.Unpooled;
import net.fabricmc.api.ModInitializer;
import net.fabricmc.fabric.api.networking.v1.ServerLoginConnectionEvents;
import net.fabricmc.fabric.api.networking.v1.ServerLoginNetworking;
import net.minecraft.network.FriendlyByteBuf;
import net.minecraft.network.chat.Component;
import net.fabricmc.fabric.api.command.v2.CommandRegistrationCallback;
import static net.minecraft.commands.Commands.literal;

public class SavageProxyBridge implements ModInitializer {
    @Override
    public void onInitialize() {
        SavageProxyConfig.LOGGER.info("Savage Proxy Bridge initialized (26.1.x)");

        // Register dummy commands that the proxy intercepts, so they autocomplete correctly
        CommandRegistrationCallback.EVENT.register((dispatcher, registryAccess, environment) -> {
            dispatcher.register(literal("savage")
                .executes(context -> {
                    // Fallback message if proxy fails to intercept
                    context.getSource().sendSystemMessage(Component.literal("§c[SavageProxy] Interception failed, reached backend."));
                    return 1;
                })
            );
        });

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
                if (!SecurityManager.verifySignature(signature, data)) {
                    SavageProxyConfig.LOGGER.warn("Invalid proxy signature from connection!");
                    handler.disconnect(Component.literal("Invalid proxy signature."));
                    return;
                }

                // Unpack the verified forwarding data
                FriendlyByteBuf dataBuf = new FriendlyByteBuf(Unpooled.wrappedBuffer(data));
                ProfileForwardingData forwardingData = ProfileForwardingData.fromBuf(dataBuf);
                dataBuf.release();

                // Inject the verified profile into the login handler
                IdentityManager.injectIdentity(handler, forwardingData);

            } catch (Exception e) {
                SavageProxyConfig.LOGGER.error("Failed to process proxy forwarding", e);
                handler.disconnect(Component.literal("Forwarding error."));
            }
        });
    }
}
