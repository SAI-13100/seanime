import { TorrentSearchContainer } from "@/app/(main)/entry/_containers/torrent-search/torrent-search-container"
import { EpisodeListItem } from "@/components/shared/episode-list-item"
import { Drawer } from "@/components/ui/drawer"
import { HorizontalDraggableScroll } from "@/components/ui/horizontal-draggable-scroll"
import { Separator } from "@/components/ui/separator"
import { MediaEntry, MediaEntryDownloadEpisode } from "@/lib/server/types"
import { atom } from "jotai"
import { useAtom } from "jotai/react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import React, { useEffect } from "react"

export const torrentSearchDrawerIsOpenAtom = atom(false)
export const torrentSearchDrawerEpisodeAtom = atom<number | undefined>(undefined)

export function TorrentSearchDrawer(props: { entry: MediaEntry }) {

    const { entry } = props

    const [isOpen, setter] = useAtom(torrentSearchDrawerIsOpenAtom)
    const searchParams = useSearchParams()
    const router = useRouter()
    const pathname = usePathname()
    const mId = searchParams.get("id")
    const downloadParam = searchParams.get("download")

    useEffect(() => {
        if (!!downloadParam) {
            setter(true)
            router.replace(pathname + `?id=${mId}`)
        }
    }, [downloadParam])

    return (
        <Drawer
            open={isOpen}
            onOpenChange={() => setter(false)}
            size="xl"
            title="Search torrents"
        >
            <div
                className="bg-[url(/pattern-2.svg)] z-[0] w-full h-[10rem] absolute opacity-50 top-[-5rem] left-0 bg-no-repeat bg-right bg-contain"
            >
                <div
                    className="w-full absolute bottom-0 h-[10rem] bg-gradient-to-t from-[--background] to-transparent z-[-2]"
                />
            </div>
            <div className="relative z-[1]">
                <EpisodeList episodes={entry.downloadInfo?.episodesToDownload} />
                <TorrentSearchContainer entry={entry} />
            </div>
        </Drawer>
    )

}

function EpisodeList({ episodes }: { episodes: MediaEntryDownloadEpisode[] | undefined }) {

    if (!episodes || !episodes.length) return null

    return (
        <div className="space-y-2 mt-4">
            <h4>Missing episodes:</h4>
            <p>Episode numbers: {episodes.slice(0, 5).map(n => n.episodeNumber).join(", ")}{episodes.length > 5 ? ", ..." : ""}</p>
            <HorizontalDraggableScroll>
                {episodes.filter(Boolean).map(item => {
                    return (
                        <EpisodeListItem
                            key={item.episode + item.aniDBEpisode}
                            media={item.episode?.basicMedia as any}
                            title={item.episode?.displayTitle || ""}
                            image={item.episode?.episodeMetadata?.image}
                            episodeTitle={item?.episode?.episodeTitle}
                            description={item.episode?.absoluteEpisodeNumber !== item.episodeNumber
                                ? `(Episode ${item?.episode?.absoluteEpisodeNumber})`
                                : undefined}
                            imageContainerClassName="w-20 h-20"
                            className="flex-none w-80"
                        />
                    )
                })}
            </HorizontalDraggableScroll>
            <Separator className="!mt-4 mb-4" />
        </div>
    )

}
