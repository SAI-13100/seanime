"use client"
import { useMediaEntryBulkAction } from "@/app/(main)/(library)/_containers/bulk-actions/_lib/media-entry-bulk-actions"
import { _bulkDeleteFilesModalIsOpenAtom, BulkDeleteFilesModal } from "@/app/(main)/entry/_containers/episode-section/bulk-delete-files-modal"
import { useOpenDefaultMediaPlayer, useStartMpvPlaybackDetection } from "@/app/(main)/entry/_lib/media-player"
import { serverStatusAtom } from "@/atoms/server-status"
import { ConfirmationDialog, useConfirmationDialog } from "@/components/application/confirmation-dialog"
import { IconButton } from "@/components/ui/button"
import { DropdownMenu } from "@/components/ui/dropdown-menu"
import { useOpenMediaEntryInExplorer } from "@/lib/server/hooks"
import { MediaEntry } from "@/lib/server/types"
import { useSetAtom } from "jotai"
import { useAtomValue } from "jotai/react"
import React from "react"
import { BiDotsVerticalRounded, BiPlayCircle, BiRightArrowAlt } from "react-icons/bi"

export function EpisodeSectionDropdownMenu({ entry }: { entry: MediaEntry }) {

    const serverStatus = useAtomValue(serverStatusAtom)

    const { startDefaultMediaPlayer } = useOpenDefaultMediaPlayer()
    const { openEntryInExplorer } = useOpenMediaEntryInExplorer()
    const { startMpvPlaybackDetection } = useStartMpvPlaybackDetection()

    const { unmatchAll, isPending } = useMediaEntryBulkAction(entry.mediaId)

    const confirmDeleteFiles = useConfirmationDialog({
        title: "Unmatch all files",
        description: "Are you sure you want to unmatch all files?",
        onConfirm: () => {
            unmatchAll(entry.mediaId)
        },
    })

    const setBulkDeleteFilesModalOpen = useSetAtom(_bulkDeleteFilesModalIsOpenAtom)


    return (
        <>
            <DropdownMenu trigger={<IconButton icon={<BiDotsVerticalRounded />} intent="gray-basic" size="xl" />}>

                {serverStatus?.settings?.mediaPlayer?.defaultPlayer == "mpv" && <DropdownMenu.Item
                    onClick={startMpvPlaybackDetection}
                >
                    <BiPlayCircle />
                    Start episode detection
                </DropdownMenu.Item>}

                <DropdownMenu.Item
                    onClick={() => openEntryInExplorer(entry.mediaId)}
                >
                    Open folder
                </DropdownMenu.Item>
                {serverStatus?.settings?.mediaPlayer?.defaultPlayer != "mpv" && <DropdownMenu.Item
                    onClick={startDefaultMediaPlayer}
                >
                    Start video player
                </DropdownMenu.Item>}
                <DropdownMenu.Separator />
                <DropdownMenu.Group title="Bulk actions">
                    {/*<DropdownMenu.Item*/}
                    {/*    onClick={bulkOffsetEpisodeModal.toggle}*/}
                    {/*>*/}
                    {/*    Offset episode numbers*/}
                    {/*</DropdownMenu.Item>*/}
                    <DropdownMenu.Item
                        className="text-red-500 dark:text-red-200 flex justify-between"
                        onClick={confirmDeleteFiles.open}
                        disabled={isPending}
                    >
                        <span>Unmatch all files</span> <BiRightArrowAlt />
                    </DropdownMenu.Item>
                    <DropdownMenu.Item
                        className="text-red-500 dark:text-red-200 flex justify-between"
                        onClick={() => setBulkDeleteFilesModalOpen(true)}
                        disabled={isPending}
                    >
                        <span>Delete some files</span> <BiRightArrowAlt />
                    </DropdownMenu.Item>
                </DropdownMenu.Group>
            </DropdownMenu>

            {/*<BulkOffsetEpisodesModal entry={entry} open={bulkOffsetEpisodeModal.active} onOpenChange={bulkOffsetEpisodeModal.off} />*/}
            <ConfirmationDialog {...confirmDeleteFiles} />
            <BulkDeleteFilesModal entry={entry} />
        </>
    )
}
